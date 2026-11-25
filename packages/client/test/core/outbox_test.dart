import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/storage/local_db.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/expenses/data/offline_expenses_repository.dart';
import 'package:splitspends/features/expenses/data/remote_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/groups/data/offline_groups_repository.dart';
import 'package:splitspends/features/groups/data/remote_groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';

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

  test('a group created offline queues and replays like expenses', () async {
    final groups = OfflineGroupsRepository(
      RemoteGroupsRepository(api()),
      outbox,
      me: () => const User(id: 'me', name: 'Me', defaultCurrency: 'EUR'),
    );
    await expectLater(
      groups.create(const GroupInput(name: ' ')),
      throwsA(predicate((e) => e.toString().contains('Give the group a name'))),
    );
    expect(await outbox.pending(), isEmpty);

    final trip = await groups.create(const GroupInput(name: 'Trip'));
    expect(trip.defaultCurrency, 'EUR');
    final op = (await outbox.pending()).single;
    expect(
      (op.method, op.path, op.label),
      ('POST', '/groups', 'Create “Trip”'),
    );
    expect((op.body! as Map)['id'], trip.id);

    online = true;
    final result = await outbox.flush(api());
    expect(result.sent, 1);
    expect(seen, ['POST /api/v1/groups']);
  });

  test('invalid input is rejected before it reaches the queue', () async {
    final repo = OfflineExpensesRepository(
      RemoteExpensesRepository(api()),
      outbox,
      meId: () => 'me',
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

  test('undo after a queued delete keeps the expense', () async {
    var deleted = false;
    final client = ApiClient(
      serverUrl: 'http://server.test',
      token: 't',
      cache: _Cache(),
      httpClient: MockClient((request) async {
        if (!online) throw http.ClientException('offline');
        final path = request.url.path;
        if (request.method == 'DELETE') deleted = true;
        if (path.endsWith('/restore')) deleted = false;
        return http.Response(
          jsonEncode({
            'data': {
              'id': 'e0',
              'description': 'Rent',
              'currency': 'EUR',
              'total': 1000,
              'date': '2026-11-01',
              'deleted': deleted,
            },
          }),
          request.method == 'DELETE' ? 204 : 200,
        );
      }),
    );
    final repo = OfflineExpensesRepository(
      RemoteExpensesRepository(client),
      outbox,
      meId: () => 'me',
    );
    online = true;
    await repo.get('e0');

    online = false;
    await repo.delete('e0');
    online = true;
    final restored = await repo.restore('e0');
    expect(restored.deleted, isFalse);
    expect(deleted, isFalse, reason: 'the delete has not been sent yet');

    final result = await outbox.flush(client);
    expect(result.sent, 2);
    expect(deleted, isFalse);
    expect(await outbox.pending(), isEmpty);

    // Offline, Undo is queued too, behind the delete.
    online = false;
    await repo.delete('e0');
    expect((await repo.restore('e0')).deleted, isFalse);
    expect(
      [for (final op in await outbox.pending()) op.method],
      ['DELETE', 'POST'],
    );
    expect((await repo.get('e0')).deleted, isFalse);
  });

  group('offline reads show queued writes', () {
    late _Cache cache;
    late ApiClient client;
    late OfflineExpensesRepository expenses;
    late OfflineGroupsRepository groups;

    const flat = 'g1';
    final rent = {
      'id': 'e0',
      'groupId': flat,
      'description': 'Rent',
      'currency': 'EUR',
      'total': 90000,
      'date': '2026-11-01',
      'shares': [
        {'userId': 'me', 'paid': 90000, 'owed': 45000},
        {'userId': 'sam', 'paid': 0, 'owed': 45000},
      ],
    };
    const taxi = ExpenseInput(
      groupId: flat,
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

    setUp(() async {
      cache = _Cache();
      client = ApiClient(
        serverUrl: 'http://server.test',
        token: 't',
        cache: cache,
        httpClient: MockClient((request) async {
          if (!online) throw http.ClientException('offline');
          final path = request.url.path;
          final query = request.url.queryParameters;
          if (path.endsWith('/expenses')) {
            final found = query['groupId'] == flat && query['q'] == null;
            return http.Response(
              jsonEncode({
                'items': [if (found) rent],
                'total': found ? 1 : 0,
              }),
              200,
            );
          }
          if (path.endsWith('/expenses/e0')) {
            return http.Response(jsonEncode({'data': rent}), 200);
          }
          final archived = query['archived'] == 'true';
          return http.Response(
            jsonEncode({
              'items': [
                if (!archived) {'id': flat, 'name': 'Flat'},
              ],
            }),
            200,
          );
        }),
      );
      expenses = OfflineExpensesRepository(
        RemoteExpensesRepository(client),
        outbox,
        meId: () => 'me',
      );
      groups = OfflineGroupsRepository(
        RemoteGroupsRepository(client),
        outbox,
        me: () => const User(id: 'me', name: 'Me'),
      );
      // Seen once while online, so the API client has cached copies.
      online = true;
      for (final query in const [
        ExpenseQuery(groupId: flat),
        ExpenseQuery(groupId: flat, q: 'TAX'),
        ExpenseQuery(groupId: 'g2'),
      ]) {
        await expenses.list(query);
      }
      await expenses.get('e0');
      await groups.list();
      await groups.list(archived: true);
      online = false;
    });

    Future<List<String>> listed(ExpenseQuery query) async {
      final page = await expenses.list(query);
      expect(page.total, page.items.length);
      return [for (final e in page.items) e.description];
    }

    test('a queued create is listed and can be opened', () async {
      final created = await expenses.create(taxi);

      expect(await listed(const ExpenseQuery(groupId: flat)), ['Taxi', 'Rent']);
      expect(await listed(const ExpenseQuery(groupId: 'g2')), isEmpty);
      expect(await listed(const ExpenseQuery(groupId: flat, q: 'TAX')), [
        'Taxi',
      ]);
      final page = await expenses.list(const ExpenseQuery(groupId: flat));
      expect(page.items.first.pending, isTrue);
      expect(page.items.last.pending, isFalse);

      final opened = await expenses.get(created.id);
      expect(opened.description, 'Taxi');
      expect(opened.myBalance, 600);
    });

    test('a queued edit and payment are shown', () async {
      await expenses.update(
        'e0',
        const ExpenseInput(
          groupId: flat,
          description: 'Rent (November)',
          currency: 'EUR',
          total: 80000,
          date: '2026-11-01',
          payers: [Payer(userId: 'me', amount: 80000)],
          splits: [
            SplitEntry(userId: 'me'),
            SplitEntry(userId: 'sam'),
          ],
        ),
      );
      await expenses.recordPayment(
        groupId: flat,
        fromUserId: 'sam',
        toUserId: 'me',
        amount: 40000,
        currency: 'EUR',
        date: '2026-11-25',
      );

      final page = await expenses.list(const ExpenseQuery(groupId: flat));
      expect(
        [for (final e in page.items) e.description],
        ['Payment', 'Rent (November)'],
      );
      expect(page.items.first.isPayment, isTrue);
      expect((await expenses.get('e0')).total, 80000);
    });

    test('a queued delete hides the expense', () async {
      await expenses.delete('e0');

      expect(await listed(const ExpenseQuery(groupId: flat)), isEmpty);
      final opened = await expenses.get('e0');
      expect(opened.deleted, isTrue);
      expect(opened.pending, isTrue);
    });

    test('a queued group create is listed and can be opened', () async {
      final created = await groups.create(const GroupInput(name: 'Trip'));

      expect([for (final g in await groups.list()) g.name], ['Trip', 'Flat']);
      expect(await groups.list(archived: true), isEmpty);
      final opened = await groups.get(created.id);
      expect(opened.name, 'Trip');
      expect([for (final m in opened.members) m.id], ['me']);

      // Its expenses too, although the server has never listed them.
      await expenses.create(
        ExpenseInput(
          groupId: created.id,
          description: taxi.description,
          currency: taxi.currency,
          total: taxi.total,
          date: taxi.date,
          payers: taxi.payers,
          splits: taxi.splits,
        ),
      );
      expect(await listed(ExpenseQuery(groupId: created.id)), ['Taxi']);
    });
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

/// Keeps GET responses in memory.
class _Cache implements ResponseCache {
  final _entries = <String, Object?>{};

  @override
  Future<Object?> read(String key) async => _entries[key];

  @override
  Future<void> write(String key, Object? body) async => _entries[key] = body;

  @override
  Future<void> clear() async => _entries.clear();
}
