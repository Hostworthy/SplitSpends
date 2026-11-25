import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/account/data/local_account_repository.dart';
import 'package:splitspends/features/activity/data/local_activity_repository.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/friends/data/local_friends_repository.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends_ledger/ledger.dart';

void main() {
  late LocalLedger ledger;
  var now = DateTime.utc(2026, 11, 24, 12);
  const meId = '00000000-0000-4000-8000-00000000000a';
  const me = Me(id: meId, name: 'Ada', defaultCurrency: 'EUR');

  setUp(() async {
    final db = await databaseFactoryMemory.openDatabase(
      'test-${DateTime.now().microsecondsSinceEpoch}',
    );
    await Stores.meta.record('profile').put(db, me.toJson());
    now = DateTime.utc(2026, 11, 24, 12);
    ledger = LocalLedger(() async => db, clock: () => now);
  });

  test('people, group, expense, payment and balances', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final friends = LocalFriendsRepository(ledger);
    final balances = LocalBalancesRepository(ledger);

    final group = await groups.create(
      const GroupInput(
        name: 'Lisbon',
        members: [
          MemberInput(name: 'Sam'),
          MemberInput(email: 'alex@x.com'),
        ],
      ),
    );
    expect(group.members.map((m) => m.displayName), ['Ada', 'Sam', 'alex']);
    final sam = group.members[1].id, alex = group.members[2].id;

    await expenses.create(
      ExpenseInput(
        groupId: group.id,
        description: 'Dinner',
        currency: 'EUR',
        total: 9000,
        date: '2026-11-20',
        payers: const [Payer(userId: meId, amount: 9000)],
        splits: [
          const SplitEntry(userId: meId),
          SplitEntry(userId: sam),
          SplitEntry(userId: alex),
        ],
      ),
    );
    var mine = await balances.mine();
    expect(mine.balances.single.amount, 6000);
    expect((await groups.get(group.id)).balances.single.amount, 6000);

    final debts = (await groups.balances(group.id)).debts;
    expect(debts.map((d) => (d.from, d.to, d.amount)).toSet(), {
      (sam, meId, 3000),
      (alex, meId, 3000),
    });

    await expenses.recordPayment(
      groupId: group.id,
      fromUserId: sam,
      toUserId: meId,
      amount: 3000,
      currency: 'EUR',
    );
    mine = await balances.mine();
    expect(mine.balances.single.amount, 3000);
    final samView = await friends.get(sam);
    expect(samView.balances, isEmpty);

    final feed = await expenses.list(ExpenseQuery(groupId: group.id));
    expect(feed.total, 2);
    expect(feed.items.first.isPayment, isTrue);

    final activity = await LocalActivityRepository(ledger).list();
    expect(activity.items.first.summary, contains('recorded a payment'));
    expect(
      (await LocalActivityRepository(ledger)
              .list(expenseId: feed.items.last.id))
          .items
          .single
          .summary,
      'You added “Dinner” in Lisbon',
    );

    // Settled members can leave; unsettled ones cannot.
    await groups.removeMember(group.id, sam);
    expect(() => groups.removeMember(group.id, alex), throwsA(anything));
  });

  test('validation errors use the API field names', () async {
    final expenses = LocalExpensesRepository(ledger);
    final friends = LocalFriendsRepository(ledger);
    final sam = (await friends.add(name: 'Sam')).user.id;
    expect(
      () => expenses.create(
        ExpenseInput(
          description: 'Taxi',
          currency: 'EUR',
          total: 1000,
          date: '2026-11-20',
          splitMethod: SplitMethod.percent,
          payers: const [Payer(userId: meId, amount: 1000)],
          splits: [
            const SplitEntry(userId: meId, value: '50'),
            SplitEntry(userId: sam, value: '40'),
          ],
        ),
      ),
      throwsA(
        predicate((e) => e.toString().contains('Percentages must add up')),
      ),
    );
  });

  test('recurring expenses catch up like the server', () async {
    final expenses = LocalExpensesRepository(ledger);
    final sam = (await LocalFriendsRepository(ledger).add(name: 'Sam')).user.id;
    await expenses.create(
      ExpenseInput(
        description: 'Rent',
        currency: 'EUR',
        total: 1000,
        date: '2026-09-01',
        recurrence: Recurrence.monthly,
        payers: const [Payer(userId: meId, amount: 1000)],
        splits: [
          const SplitEntry(userId: meId),
          SplitEntry(userId: sam),
        ],
      ),
    );
    expect(await ledger.runRecurring(), 2); // Oct 1 and Nov 1
    final list = await expenses.list(const ExpenseQuery());
    expect(list.items.map((e) => e.date), [
      '2026-11-01',
      '2026-10-01',
      '2026-09-01',
    ]);
    expect(list.items.first.recurrence, Recurrence.monthly);
    expect(list.items.last.recurrence, Recurrence.none);
    expect(await ledger.runRecurring(), 0);
  });

  test('a series that fell behind books only the latest weeks', () async {
    final expenses = LocalExpensesRepository(ledger);
    final sam = (await LocalFriendsRepository(ledger).add(name: 'Sam')).user.id;
    await expenses.create(
      ExpenseInput(
        description: 'Cleaning',
        currency: 'EUR',
        total: 1000,
        date: '2024-11-26',
        recurrence: Recurrence.weekly,
        payers: const [Payer(userId: meId, amount: 1000)],
        splits: [
          const SplitEntry(userId: meId),
          SplitEntry(userId: sam),
        ],
      ),
    );
    expect(await ledger.runRecurring(), catchUp);
    final list = await expenses.list(const ExpenseQuery());
    expect(list.total, catchUp + 1);
    expect(list.items.first.date, '2026-11-24');
    expect(
      list.items.where((e) => e.recurrence != Recurrence.none),
      hasLength(1),
    );
  });

  test('a group series stops once someone in it leaves', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await groups.create(
      const GroupInput(
        name: 'Flat',
        members: [MemberInput(name: 'Sam')],
      ),
    );
    final sam = group.members[1].id;
    final rent = await expenses.create(
      ExpenseInput(
        groupId: group.id,
        description: 'Rent',
        currency: 'EUR',
        total: 1000,
        date: '2026-11-01',
        recurrence: Recurrence.monthly,
        payers: const [Payer(userId: meId, amount: 1000)],
        splits: [
          const SplitEntry(userId: meId),
          SplitEntry(userId: sam),
        ],
      ),
    );
    await expenses.recordPayment(
      groupId: group.id,
      fromUserId: sam,
      toUserId: meId,
      amount: 500,
      currency: 'EUR',
    );
    await groups.removeMember(group.id, sam);
    now = DateTime.utc(2026, 12, 2, 12);
    expect(await ledger.runRecurring(), 0);
    expect((await expenses.get(rent.id)).recurrence, Recurrence.none);
    final feed = await LocalActivityRepository(ledger).list(expenseId: rent.id);
    expect(
      feed.items.first.summary,
      '“Rent” stopped repeating in Flat because someone in it left the group',
    );
  });
}
