// Opt-in end-to-end check against a running backend:
//
//   SPLITSPENDS_E2E_URL=http://localhost:8080 \
//   SPLITSPENDS_E2E_LOG=/path/to/server.log \
//   flutter test test/e2e
//
// Builds data on-device, imports it into a fresh account, and checks that
// the server computes exactly the balances the local ledger computed.
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/account/data/local_account_repository.dart';
import 'package:splitspends/features/categories/data/local_categories_repository.dart';
import 'package:splitspends/features/categories/data/remote_categories_repository.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/data/offline_expenses_repository.dart';
import 'package:splitspends/features/expenses/data/remote_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/groups/data/remote_groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/sync/data/import_service.dart';

Object? _snacks;

void main() {
  final url = Platform.environment['SPLITSPENDS_E2E_URL'];
  final log = Platform.environment['SPLITSPENDS_E2E_LOG'];
  final skip = url == null || log == null
      ? 'Set SPLITSPENDS_E2E_URL and SPLITSPENDS_E2E_LOG to run'
      : null;

  Map<String, int> flat(List<Balance> list) => {
    for (final b in list) b.currency: b.amount,
  };

  test('import keeps every balance identical', skip: skip, () async {
    final client = http.Client();
    final email = 'e2e-${DateTime.now().microsecondsSinceEpoch}@example.com';

    // 1. Sign in (the server logs codes when SMTP is not configured).
    final anon = ApiClient(serverUrl: url!, httpClient: client);
    await anon.post('/auth/otp/request', {'email': email});
    await Future<void>.delayed(const Duration(milliseconds: 300));
    final code = RegExp(
      '${RegExp.escape(email)}.*?\\b(\\d{6})\\b|\\b(\\d{6})\\b.*?${RegExp.escape(email)}',
    ).allMatches(File(log!).readAsStringSync()).last;
    final login = await anon.postData('/auth/otp/verify', {
      'email': email,
      'code': code[1] ?? code[2],
    }, (d) => d as Map<String, dynamic>);
    final token = login['token'] as String;
    final serverMe = Me.fromJson(login['user'] as Map<String, dynamic>);
    final api = ApiClient(serverUrl: url, token: token, httpClient: client);

    // 2. Build on-device data with every split method.
    final db = await databaseFactoryMemory.openDatabase('e2e');
    const localMe = Me(
      id: '11111111-1111-4111-8111-111111111111',
      name: 'Ada',
      defaultCurrency: 'EUR',
    );
    await Stores.meta.record('profile').put(db, localMe.toJson());
    final ledger = LocalLedger(() async => db);
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final g = await groups.create(
      const GroupInput(
        name: 'Trip',
        defaultCurrency: 'EUR',
        simplifyDebts: false,
        members: [
          MemberInput(name: 'Sam'),
          MemberInput(name: 'Kim', email: 'kim-e2e@example.com'),
          MemberInput(name: 'Lee'),
        ],
      ),
    );
    final ids = [for (final m in g.members) m.id];
    final me = localMe.id, sam = ids[1], kim = ids[2], lee = ids[3];
    Future<void> add(ExpenseInput input) => expenses.create(input);
    await add(
      ExpenseInput(
        groupId: g.id,
        description: 'Hotel',
        currency: 'EUR',
        total: 100001,
        date: '2026-11-01',
        payers: [Payer(userId: me, amount: 100001)],
        splits: [for (final id in ids) SplitEntry(userId: id)],
      ),
    );
    await add(
      ExpenseInput(
        groupId: g.id,
        description: 'Fuel',
        currency: 'EUR',
        total: 9999,
        date: '2026-11-02',
        splitMethod: SplitMethod.percent,
        payers: [
          Payer(userId: sam, amount: 5000),
          Payer(userId: kim, amount: 4999),
        ],
        splits: [
          SplitEntry(userId: me, value: '33.33'),
          SplitEntry(userId: sam, value: '33.33'),
          SplitEntry(userId: lee, value: '33.34'),
        ],
      ),
    );
    await add(
      ExpenseInput(
        groupId: g.id,
        description: 'Dinner',
        currency: 'EUR',
        total: 7350,
        date: '2026-11-03',
        splitMethod: SplitMethod.itemized,
        payers: [Payer(userId: lee, amount: 7350)],
        items: [
          LineItem(name: 'Fish', amount: 3100, userIds: [me, kim]),
          LineItem(name: 'Wine', amount: 3001, userIds: [me, sam, lee]),
        ],
        tax: 749,
        tip: 500,
      ),
    );
    await add(
      ExpenseInput(
        groupId: g.id,
        description: 'Boat',
        currency: 'USD',
        total: 5000,
        date: '2026-11-04',
        splitMethod: SplitMethod.shares,
        payers: [Payer(userId: kim, amount: 5000)],
        splits: [
          SplitEntry(userId: me, value: '2'),
          SplitEntry(userId: sam, value: '1.5'),
          SplitEntry(userId: kim, value: '1'),
        ],
      ),
    );
    await add(
      ExpenseInput(
        description: 'Taxi',
        currency: 'EUR',
        total: 3001,
        date: '2026-11-05',
        splitMethod: SplitMethod.adjustment,
        payers: [Payer(userId: sam, amount: 3001)],
        splits: [
          SplitEntry(userId: me, value: '500'),
          SplitEntry(userId: sam, value: '-200'),
        ],
      ),
    );
    await expenses.recordPayment(
      groupId: g.id,
      fromUserId: sam,
      toUserId: me,
      amount: 1234,
      currency: 'EUR',
      date: '2026-11-06',
    );
    // In a group, members book payments and expenses for each other.
    await expenses.recordPayment(
      groupId: g.id,
      fromUserId: kim,
      toUserId: lee,
      amount: 777,
      currency: 'EUR',
      date: '2026-11-06',
    );
    await add(
      ExpenseInput(
        groupId: g.id,
        description: 'Ferry',
        currency: 'EUR',
        total: 1001,
        date: '2026-11-06',
        payers: [Payer(userId: lee, amount: 1001)],
        splits: [
          SplitEntry(userId: sam),
          SplitEntry(userId: kim),
        ],
      ),
    );
    // A default split and custom categories, one of them deleted but still
    // used by an expense.
    await groups.update(
      g.id,
      GroupInput(
        defaultSplit: DefaultSplit(
          method: SplitMethod.shares,
          values: {me: '2', sam: '1', kim: '1', lee: '0'},
        ),
      ),
    );
    final categories = LocalCategoriesRepository(ledger);
    final boats = await categories.create(
      const CategoryInput(name: 'Boats', icon: 'attractions', group: 'Travel'),
    );
    final old = await categories.create(const CategoryInput(name: 'Old'));
    for (final c in [boats, old]) {
      await add(
        ExpenseInput(
          groupId: g.id,
          description: 'Kayak ${c.name}',
          currency: 'EUR',
          total: 4000,
          date: '2026-11-06',
          category: c.id,
          payers: [Payer(userId: me, amount: 4000)],
          splits: [
            SplitEntry(userId: me),
            SplitEntry(userId: sam),
          ],
        ),
      );
    }
    await categories.delete(old.id);
    final localTotals = await LocalBalancesRepository(ledger).mine();
    final localGroup = await groups.balances(g.id);

    // 3. Import, then compare with the server's own computation.
    final import = ImportService(ledger, client);
    await import.upload(serverUrl: url, token: token);
    await import.upload(serverUrl: url, token: token); // retry is safe

    final serverTotals = await api.getData(
      '/me/balances',
      (d) => BalanceSummary.fromJson(d as Map<String, dynamic>),
    );
    expect(flat(serverTotals.balances), flat(localTotals.balances));
    expect(flat(serverTotals.owed), flat(localTotals.owed));
    expect(flat(serverTotals.owing), flat(localTotals.owing));

    final serverGroup = await api.getData(
      '/groups/${g.id}/balances',
      (d) => GroupBalances.fromJson(d as Map<String, dynamic>),
    );
    // After import the local ids were rewritten with the server's map.
    final mapped = await groups.balances(g.id);
    String debts(GroupBalances b) =>
        [for (final d in b.debts) '${d.from}>${d.to}:${d.amount}${d.currency}']
            .join(' ');
    expect(debts(serverGroup), debts(mapped));
    expect(serverGroup.debts.length, localGroup.debts.length);
    final profile = await LocalAccountRepository(ledger).me();
    expect(profile.id, serverMe.id, reason: 'local me maps to the account');

    // The default split arrives with server ids, matching the local copy.
    final serverSplit = (await RemoteGroupsRepository(api).get(g.id))
        .defaultSplit;
    final localSplit = (await groups.get(g.id)).defaultSplit;
    expect(serverSplit?.method, SplitMethod.shares);
    expect(serverSplit?.values, localSplit?.values);
    expect(serverSplit?.values[serverMe.id], '2');
    // Custom categories keep their ids; deleted ones stay on expenses only.
    final serverCategories = {
      for (final c in await RemoteCategoriesRepository(api).list()) c.id: c,
    };
    expect(serverCategories[boats.id]?.name, 'Boats');
    expect(serverCategories[boats.id]?.mine, isTrue);
    expect(serverCategories.containsKey(old.id), isFalse);
    final kayaks = await api.getList(
      '/expenses',
      Expense.fromJson,
      query: {'groupId': g.id, 'q': 'Kayak'},
    );
    expect({for (final e in kayaks.items) e.category}, {boats.id, old.id});

    // 4. Offline outbox replays with client ids exactly once.
    final outbox = Outbox(
      () async => db,
      owner: (server: 'http://127.0.0.1:9', user: serverMe.id),
    );
    final offline = OfflineExpensesRepository(
      RemoteExpensesRepository(
        ApiClient(
          serverUrl: 'http://127.0.0.1:9',
          token: token,
          httpClient: client,
        ),
      ),
      outbox,
      meId: () => serverMe.id,
    );
    final members = [for (final m in (await groups.get(g.id)).members) m.id];
    final queued = await offline.create(
      ExpenseInput(
        groupId: g.id,
        description: 'Snacks',
        currency: 'EUR',
        total: 900,
        date: '2026-11-07',
        payers: [Payer(userId: serverMe.id, amount: 900)],
        splits: [for (final id in members) SplitEntry(userId: id)],
      ),
    );
    expect((await outbox.pending()).length, 1);
    _snacks = (await outbox.pending()).single.body;
    expect((await outbox.flush(api)).sent, 1);
    // Replaying the same create again is idempotent on the server.
    await outbox.add('POST', '/expenses', body: _snacks, label: 'again');
    expect((await outbox.flush(api)).rejected, isEmpty);
    final again = await api.getList(
      '/expenses',
      Expense.fromJson,
      query: {'groupId': g.id, 'q': 'Snacks'},
    );
    expect(again.items.single.id, queued.id);
  });
}
