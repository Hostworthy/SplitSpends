import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/expenses/data/offline_expenses_repository.dart';
import 'package:splitspends/features/expenses/data/remote_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

void main() {
  late Outbox outbox;
  var online = false;
  final seen = <String>[];

  setUp(() async {
    final db = await databaseFactoryMemory.openDatabase(
      'outbox-${DateTime.now().microsecondsSinceEpoch}',
    );
    outbox = Outbox(() async => db);
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
}
