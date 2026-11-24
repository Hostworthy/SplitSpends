import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/storage/local_db.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/expenses/data/offline_expenses_repository.dart';
import 'package:splitspends/features/expenses/data/remote_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

const _ada = (server: 'http://server.test', user: 'ada');

void main() {
  late Database db;
  late Outbox outbox;
  var online = false;
  final seen = <String>[];

  setUp(() async {
    db = await databaseFactoryMemory.openDatabase(
      'outbox-${DateTime.now().microsecondsSinceEpoch}',
    );
    outbox = Outbox(() async => db, owner: _ada);
    online = false;
    seen.clear();
  });

  ApiClient api() => ApiClient(
    serverUrl: 'http://server.test',
    token: 't',
    httpClient: MockClient((request) async {
      if (!online) throw http.ClientException('offline');
      seen.add('${request.method} ${request.url.path}');
      if (request.url.path.endsWith('/payments')) {
        return http.Response(
          jsonEncode({'error': 'Not a member.', 'code': 'VALIDATION_ERROR'}),
          422,
        );
      }
      if (request.method == 'DELETE') return http.Response('', 204);
      final body = jsonDecode(request.body) as Map<String, dynamic>;
      return http.Response(
        jsonEncode({
          'data': {
            'id': body['id'],
            'description': body['description'] ?? 'Payment',
            'currency': 'EUR',
            'total': body['total'] ?? body['amount'],
            'date': '2026-11-24',
            'shares': <Object>[],
          },
        }),
        201,
      );
    }),
  );

  test('offline writes queue with client ids and replay in order', () async {
    final repo = OfflineExpensesRepository(
      RemoteExpensesRepository(api()),
      outbox,
      meId: () => 'me',
      onQueued: () {},
    );
    const input = ExpenseInput(
      description: 'Taxi',
      currency: 'EUR',
      total: 1200,
      date: '2026-11-24',
      payers: [Payer(userId: 'me', amount: 1200)],
      splits: [
        SplitEntry(userId: 'me'),
        SplitEntry(userId: 'sam'),
      ],
    );

    final created = await repo.create(input);
    expect(created.myBalance, 600, reason: 'optimistic result uses the ledger');
    await repo.recordPayment(
      fromUserId: 'sam',
      toUserId: 'me',
      amount: 600,
      currency: 'EUR',
    );
    expect((await outbox.pending()).length, 2);

    // Still offline: nothing is lost.
    var result = await outbox.flush(api());
    expect(result.offline, isTrue);
    expect((await outbox.pending()).length, 2);

    online = true;
    // While older writes wait, new writes queue behind them.
    await repo.delete(created.id);
    expect((await outbox.pending()).length, 3);

    result = await outbox.flush(api());
    expect(seen, [
      'POST /api/v1/expenses',
      'POST /api/v1/payments',
      'DELETE /api/v1/expenses/${created.id}',
    ]);
    expect(result.sent, 2);
    expect(result.rejected.single, contains('Not a member.'));
    expect(await outbox.pending(), isEmpty);
  });

  test('invalid input is rejected before it reaches the queue', () async {
    final repo = OfflineExpensesRepository(
      RemoteExpensesRepository(api()),
      outbox,
      meId: () => 'me',
      onQueued: () {},
    );
    await expectLater(
      repo.create(
        const ExpenseInput(
          description: 'Bad',
          currency: 'EUR',
          total: 1000,
          date: '2026-11-24',
          payers: [Payer(userId: 'me', amount: 900)],
          splits: [SplitEntry(userId: 'me')],
        ),
      ),
      throwsA(predicate((e) => e.toString().contains('add up to the total'))),
    );
    expect(await outbox.pending(), isEmpty);
  });

  group('ownership', () {
    test(
      'writes replay only for the account and server that made them',
      () async {
        await outbox.add(
          'POST',
          '/expenses',
          body: {'id': 'e1'},
          label: 'Taxi',
        );
        online = true;

        for (final other in [
          (server: 'http://server.test', user: 'bob'),
          (server: 'http://other.test', user: 'ada'),
        ]) {
          final foreign = Outbox(() async => db, owner: other);
          expect(await foreign.pending(), isEmpty);
          expect(await foreign.others().first, 1);
          final result = await foreign.flush(api());
          expect(result.sent, 0);
          await foreign.clear();
        }
        expect(seen, isEmpty);
        expect(await Outbox(() async => db, owner: null).others().first, 1);

        final result = await outbox.flush(api());
        expect(result.sent, 1);
        expect(seen, ['POST /api/v1/expenses']);
        expect(await outbox.pending(), isEmpty);
      },
    );

    test('queueing needs a signed-in account', () async {
      final nobody = Outbox(() async => db, owner: null);
      await expectLater(
        nobody.add('POST', '/expenses', label: 'Taxi'),
        throwsStateError,
      );
    });

    test('claim adopts writes queued before the account was known', () async {
      // Queued by an earlier version, without an owner.
      await Stores.outbox.add(db, {
        'method': 'POST',
        'path': '/expenses',
        'body': {'id': 'e1'},
        'label': 'Taxi',
      });
      // Queued on an offline start before the account id was recorded.
      await Outbox(
        () async => db,
        owner: (server: _ada.server, user: null),
      ).add('POST', '/payments', body: {'id': 'p1'}, label: 'Payment');
      expect(await outbox.pending(), isEmpty);

      await outbox.claim();
      expect(
        [for (final op in await outbox.pending()) op.path],
        ['/expenses', '/payments'],
      );
    });

    test('clearOthers keeps the account own writes', () async {
      await outbox.add('POST', '/expenses', body: {'id': 'e1'}, label: 'A');
      await Outbox(
        () async => db,
        owner: (server: _ada.server, user: 'bob'),
      ).add('POST', '/expenses', body: {'id': 'e2'}, label: 'B');

      await outbox.clearOthers();
      expect(await outbox.others().first, 0);
      expect((await outbox.pending()).single.label, 'A');
    });
  });

  group('replay answers', () {
    late http.Response Function(http.Request request) answer;

    ApiClient server() => ApiClient(
      serverUrl: 'http://server.test',
      token: 't',
      httpClient: MockClient((request) async {
        seen.add('${request.method} ${request.url.path}');
        return answer(request);
      }),
    );

    http.Response envelope(int status, String code) => http.Response(
      jsonEncode({'error': 'The server says no.', 'code': code}),
      status,
    );

    Future<void> queue(String id, {String method = 'POST'}) => outbox.add(
      method,
      method == 'POST' ? '/expenses' : '/expenses/$id',
      body: {'id': id},
      label: id,
    );

    test(
      '429 keeps the write, stops the pass and passes on Retry-After',
      () async {
        await queue('e1');
        await queue('e2');
        answer = (_) => http.Response(
          'Too many requests',
          429,
          headers: {'retry-after': '30'},
        );

        final result = await outbox.flush(server());
        expect(result.offline, isTrue);
        expect(result.retryAfter, const Duration(seconds: 30));
        expect(result.rejected, isEmpty);
        expect(seen, ['POST /api/v1/expenses'], reason: 'the pass stops');
        expect((await outbox.pending()).length, 2);
      },
    );

    test('an answer without the error envelope keeps the write', () async {
      await queue('e1');
      for (final status in [403, 404, 408, 413, 502]) {
        answer = (_) => http.Response('<html>Not here</html>', status);
        final result = await outbox.flush(server());
        expect(result.offline, isTrue, reason: '$status');
        expect(result.rejected, isEmpty, reason: '$status');
      }
      final op = (await outbox.pending()).single;
      expect(op.attempts, 0, reason: 'proxy answers are not server failures');
      expect(op.stuck, isFalse);
    });

    test('a domain verdict drops the write and goes on', () async {
      await queue('e1');
      await queue('e2');
      answer = (request) => request.body.contains('e1')
          ? envelope(404, 'NOT_FOUND')
          : http.Response(jsonEncode({'data': null}), 201);

      final result = await outbox.flush(server());
      expect(result.rejected.single, 'e1: The server says no.');
      expect(result.sent, 1);
      expect(await outbox.pending(), isEmpty);
    });

    test(
      'a write the server keeps failing on gets stuck, not in the way',
      () async {
        await queue('e1');
        await queue('e1', method: 'PUT');
        await queue('e2');
        answer = (request) => request.body.contains('"e1"')
            ? envelope(500, 'INTERNAL')
            : http.Response(jsonEncode({'data': null}), 201);

        for (var pass = 1; pass < Outbox.stuckAfter; pass++) {
          final result = await outbox.flush(server());
          expect(result.offline, isTrue);
          expect(result.sent, 0);
          expect((await outbox.pending()).first.attempts, pass);
        }
        seen.clear();
        var result = await outbox.flush(server());
        expect(seen, ['POST /api/v1/expenses', 'POST /api/v1/expenses']);
        expect(result.sent, 1, reason: 'the next write is still sent');
        final waiting = await outbox.pending();
        expect([for (final op in waiting) op.method], ['POST', 'PUT']);
        expect(waiting.first.stuck, isTrue);
        expect(waiting.first.error, 'The server says no.');
        expect(await outbox.stuck().first, hasLength(1));

        // The edit of the stuck expense waits behind it.
        seen.clear();
        result = await outbox.flush(server());
        expect(seen, isEmpty);

        // Retry once the server is fixed: both are sent, in order.
        answer = (_) => http.Response(jsonEncode({'data': null}), 200);
        await outbox.unstick();
        result = await outbox.flush(server());
        expect(seen, ['POST /api/v1/expenses', 'PUT /api/v1/expenses/e1']);
        expect(await outbox.pending(), isEmpty);
      },
    );

    test('discard removes one write', () async {
      await queue('e1');
      await queue('e2');
      await outbox.discard((await outbox.pending()).first.key);
      expect((await outbox.pending()).single.label, 'e2');
    });
  });
}
