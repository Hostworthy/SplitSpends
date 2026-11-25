import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/account/data/local_account_repository.dart';
import 'package:splitspends/features/activity/data/local_activity_repository.dart';
import 'package:splitspends/features/categories/data/local_categories_repository.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/optimistic.dart';
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
    await expectLater(
      groups.removeMember(group.id, alex),
      throwsA(isA<ApiError>().having((e) => e.status, 'status', 409)),
    );
  });

  Matcher apiError(String field, String message) => isA<ApiError>()
      .having((e) => e.field, 'field', field)
      .having((e) => e.message, 'message', message);

  Future<Group> trip(LocalGroupsRepository groups) => groups.create(
    const GroupInput(
      name: 'Trip',
      members: [
        MemberInput(name: 'Sam'),
        MemberInput(name: 'Alex'),
      ],
    ),
  );

  test('adding someone twice is a conflict, like on the server', () async {
    final groups = LocalGroupsRepository(ledger);
    final group = await groups.create(
      const GroupInput(
        name: 'Trip',
        members: [MemberInput(email: 'sam@example.com', name: 'Sam')],
      ),
    );
    await expectLater(
      groups.addMember(group.id, email: 'sam@example.com'),
      throwsA(
        isA<ApiError>()
            .having((e) => e.status, 'status', 409)
            .having(
              (e) => e.message,
              'message',
              'Sam is already in this group.',
            ),
      ),
    );
    expect((await groups.get(group.id)).members, hasLength(2));
  });

  test('removing someone who is not a member is not found', () async {
    final groups = LocalGroupsRepository(ledger);
    final group = await trip(groups);
    final activity = LocalActivityRepository(ledger);
    final before = (await activity.list()).total;
    await expectLater(
      groups.removeMember(group.id, '00000000-0000-4000-8000-0000000000ff'),
      throwsA(
        isA<ApiError>().having(
          (e) => e.message,
          'message',
          'Member not found.',
        ),
      ),
    );
    expect((await activity.list()).total, before);
    expect((await groups.get(group.id)).members, hasLength(3));
  });

  test('the device owner cannot leave a group', () async {
    final groups = LocalGroupsRepository(ledger);
    final group = await trip(groups);
    await expectLater(
      groups.removeMember(group.id, meId),
      throwsA(isA<ApiError>().having((e) => e.status, 'status', 409)),
    );
    expect((await groups.list()).map((g) => g.id), [group.id]);
  });

  test('a group member can record a payment between two others', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id, alex = group.members[2].id;
    final payment = await expenses.recordPayment(
      groupId: group.id,
      fromUserId: sam,
      toUserId: alex,
      amount: 1500,
      currency: 'EUR',
    );
    expect(payment.isPayment, isTrue);
    expect(payment.myBalance, 0);
    final debts = (await groups.balances(group.id)).debts;
    expect(debts.map((d) => (d.from, d.to, d.amount)), [(alex, sam, 1500)]);
  });

  test('a group expense without me is accepted', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id, alex = group.members[2].id;
    final taxi = await expenses.create(
      ExpenseInput(
        groupId: group.id,
        description: 'Taxi',
        currency: 'EUR',
        total: 2000,
        date: '2026-11-20',
        payers: [Payer(userId: sam, amount: 2000)],
        splits: [
          SplitEntry(userId: sam),
          SplitEntry(userId: alex),
        ],
      ),
    );
    expect(taxi.myBalance, 0);
    expect((await expenses.list(ExpenseQuery(groupId: group.id))).total, 1);
  });

  test('a group expense with someone outside the group is rejected', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final kim = (await LocalFriendsRepository(ledger).add(name: 'Kim')).user.id;
    await expectLater(
      expenses.create(
        ExpenseInput(
          groupId: group.id,
          description: 'Taxi',
          currency: 'EUR',
          total: 2000,
          date: '2026-11-20',
          payers: const [Payer(userId: meId, amount: 2000)],
          splits: [
            const SplitEntry(userId: meId),
            SplitEntry(userId: kim),
          ],
        ),
      ),
      throwsA(
        apiError(
          'splits',
          'Everyone in the expense must be a member of the group.',
        ),
      ),
    );
  });

  test(
    'a non-group expense without me is rejected with field splits',
    () async {
      final expenses = LocalExpensesRepository(ledger);
      final friends = LocalFriendsRepository(ledger);
      final sam = (await friends.add(name: 'Sam')).user.id;
      final alex = (await friends.add(name: 'Alex')).user.id;
      await expectLater(
        expenses.create(
          ExpenseInput(
            description: 'Taxi',
            currency: 'EUR',
            total: 2000,
            date: '2026-11-20',
            payers: [Payer(userId: sam, amount: 2000)],
            splits: [
              SplitEntry(userId: sam),
              SplitEntry(userId: alex),
            ],
          ),
        ),
        throwsA(
          apiError('splits', 'You must be part of an expense outside a group.'),
        ),
      );
      await expectLater(
        expenses.recordPayment(
          fromUserId: sam,
          toUserId: alex,
          amount: 1500,
          currency: 'EUR',
        ),
        throwsA(
          apiError(
            'fromUserId',
            'You must be part of a payment outside a group.',
          ),
        ),
      );
      expect((await expenses.list(const ExpenseQuery())).total, 0);
    },
  );

  ExpenseInput taxi({String? groupId, required List<String> people}) =>
      ExpenseInput(
        groupId: groupId,
        description: 'Taxi',
        currency: 'EUR',
        total: 2000,
        date: '2026-11-20',
        payers: const [Payer(userId: meId, amount: 2000)],
        splits: [for (final id in people) SplitEntry(userId: id)],
      );

  test('the payment category is only for payments, like the server', () async {
    final expenses = LocalExpensesRepository(ledger);
    final sam = (await LocalFriendsRepository(ledger).add(name: 'Sam')).user.id;
    final input = taxi(people: [meId, sam]);
    await expectLater(
      expenses.create(
        ExpenseInput(
          description: input.description,
          currency: input.currency,
          total: input.total,
          date: input.date,
          category: paymentCategory,
          payers: input.payers,
          splits: input.splits,
        ),
      ),
      throwsA(
        apiError(
          'category',
          'That category is only for payments. Choose another one.',
        ),
      ),
    );
    final payment = await expenses.recordPayment(
      fromUserId: sam,
      toUserId: meId,
      amount: 1500,
      currency: 'EUR',
    );
    expect(payment.category, paymentCategory);
    final offline = optimisticPayment(
      id: 'p1',
      fromUserId: sam,
      toUserId: meId,
      amount: 1500,
      currency: 'EUR',
      meId: meId,
      today: DateTime.utc(2026, 11, 20),
    );
    expect(offline.category, paymentCategory);
  });

  test('editing a deleted expense is a conflict', () async {
    final expenses = LocalExpensesRepository(ledger);
    final sam = (await LocalFriendsRepository(ledger).add(name: 'Sam')).user.id;
    final e = await expenses.create(taxi(people: [meId, sam]));
    await expenses.delete(e.id);
    await expectLater(
      expenses.update(e.id, taxi(people: [meId, sam])),
      throwsA(
        isA<ApiError>().having(
          (e) => e.message,
          'message',
          'Restore the expense before editing it.',
        ),
      ),
    );
    expect((await expenses.list(const ExpenseQuery())).total, 0);
  });

  test('update keeps and validates the original group', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id;
    final kim = (await LocalFriendsRepository(ledger).add(name: 'Kim')).user.id;
    final e = await expenses.create(
      taxi(groupId: group.id, people: [meId, sam]),
    );
    await expectLater(
      expenses.update(e.id, taxi(people: [meId, kim])),
      throwsA(
        apiError(
          'splits',
          'Everyone in the expense must be a member of the group.',
        ),
      ),
    );
    final edited = await expenses.update(e.id, taxi(people: [meId]));
    expect(edited.groupId, group.id);
  });

  test('deleting twice records one activity', () async {
    final expenses = LocalExpensesRepository(ledger);
    final sam = (await LocalFriendsRepository(ledger).add(name: 'Sam')).user.id;
    final e = await expenses.create(taxi(people: [meId, sam]));
    await expenses.delete(e.id);
    await expenses.delete(e.id);
    await expenses.restore(e.id);
    await expenses.restore(e.id);
    final feed = await LocalActivityRepository(ledger).list(expenseId: e.id);
    expect(feed.items.map((a) => a.kind).toList()..sort(), [
      'expense.created',
      'expense.deleted',
      'expense.restored',
    ]);
  });

  test('local list filters by friend, text and category with paging', () async {
    final expenses = LocalExpensesRepository(ledger);
    final friends = LocalFriendsRepository(ledger);
    final sam = (await friends.add(name: 'Sam')).user.id;
    final kim = (await friends.add(name: 'Kim')).user.id;
    Future<Expense> add(
      String description,
      String date, {
      required String friend,
      String notes = '',
      String? category,
    }) {
      // Later entries on the same day are created later.
      now = now.add(const Duration(minutes: 1));
      return expenses.create(
        ExpenseInput(
          description: description,
          currency: 'EUR',
          total: 1000,
          date: date,
          notes: notes,
          category: category,
          payers: const [Payer(userId: meId, amount: 1000)],
          splits: [
            const SplitEntry(userId: meId),
            SplitEntry(userId: friend),
          ],
        ),
      );
    }

    await add('Taxi', '2026-11-20', friend: sam, category: 'transport.taxi');
    await add('Lunch', '2026-11-22', friend: kim, notes: 'Taxi home after');
    await add('Dinner', '2026-11-20', friend: sam, category: 'food.dining');
    final gone = await add('Taxi back', '2026-11-21', friend: kim);
    await add('Snacks', '2026-11-19', friend: sam, category: 'food.dining');
    await expenses.delete(gone.id);

    Future<void> lists(
      ExpenseQuery query,
      List<String> descriptions, {
      required int total,
      int limit = 50,
      int offset = 0,
    }) async {
      final page = await expenses.list(query, limit: limit, offset: offset);
      expect([for (final e in page.items) e.description], descriptions);
      expect(page.total, total);
    }

    // Newest date first, then the one created last on the same day.
    await lists(const ExpenseQuery(), [
      'Lunch',
      'Dinner',
      'Taxi',
      'Snacks',
    ], total: 4);
    await lists(ExpenseQuery(friendId: sam), [
      'Dinner',
      'Taxi',
      'Snacks',
    ], total: 3);
    await lists(ExpenseQuery(friendId: kim), ['Lunch'], total: 1);
    // Description or notes, ignoring case and spaces around the text.
    await lists(const ExpenseQuery(q: ' tAXi '), ['Lunch', 'Taxi'], total: 2);
    await lists(const ExpenseQuery(category: 'food.dining'), [
      'Dinner',
      'Snacks',
    ], total: 2);
    await lists(
      ExpenseQuery(friendId: sam, category: 'food.dining', q: 'snack'),
      ['Snacks'],
      total: 1,
    );
    await lists(const ExpenseQuery(), ['Lunch', 'Dinner'], total: 4, limit: 2);
    await lists(
      const ExpenseQuery(),
      ['Taxi', 'Snacks'],
      total: 4,
      limit: 2,
      offset: 2,
    );
    await lists(const ExpenseQuery(), [], total: 4, offset: 4);
  });

  test('offline and on-device results agree for every split method', () async {
    final expenses = LocalExpensesRepository(ledger);
    final friends = LocalFriendsRepository(ledger);
    final sam = (await friends.add(name: 'Sam')).user.id;
    final kim = (await friends.add(name: 'Kim')).user.id;
    for (final method in SplitMethod.values) {
      final input = ExpenseInput(
        description: 'Dinner',
        currency: 'EUR',
        total: 1000,
        date: '2026-11-20',
        category: 'food.dining',
        splitMethod: method,
        recurrence: Recurrence.monthly,
        payers: [
          const Payer(userId: meId, amount: 700),
          Payer(userId: sam, amount: 300),
        ],
        splits: [
          SplitEntry(
            userId: meId,
            value: switch (method) {
              SplitMethod.exact => '500',
              SplitMethod.percent => '50',
              SplitMethod.shares => '2',
              SplitMethod.adjustment => '100',
              _ => null,
            },
          ),
          SplitEntry(
            userId: sam,
            value: switch (method) {
              SplitMethod.exact => '200',
              SplitMethod.percent => '20',
              SplitMethod.shares => '1',
              _ => null,
            },
          ),
          SplitEntry(
            userId: kim,
            value: switch (method) {
              SplitMethod.exact => '300',
              SplitMethod.percent => '30',
              SplitMethod.shares => '1',
              _ => null,
            },
          ),
        ],
        items: [
          LineItem(name: 'Pasta', amount: 600, userIds: [meId, sam]),
          LineItem(name: 'Wine', amount: 300, userIds: [kim]),
        ],
        tax: 60,
        tip: 40,
      );
      final local = await expenses.create(input);
      final offline = optimisticExpense(
        input,
        id: local.id,
        meId: meId,
        today: ledger.today(),
      );
      List<Object?> shape(Expense e) => [
        e.kind,
        e.splitMethod,
        e.category,
        [for (final s in e.shares) (s.userId, s.paid, s.owed, s.value)],
        [for (final i in e.items) (i.name, i.amount, i.userIds.join(','))],
        e.tax,
        e.tip,
        e.recurrence,
        e.myBalance,
      ];
      expect(shape(offline), shape(local), reason: method.wire);
    }
  });

  test('restore is refused after a participant left the group', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id;
    final gone = await expenses.create(
      taxi(groupId: group.id, people: [meId, sam]),
    );
    await expenses.delete(gone.id);
    final kept = await expenses.create(
      taxi(groupId: group.id, people: [meId, sam]),
    );
    await expenses.recordPayment(
      groupId: group.id,
      fromUserId: sam,
      toUserId: meId,
      amount: 1000,
      currency: 'EUR',
    );
    await groups.removeMember(group.id, sam);
    await expectLater(
      expenses.restore(gone.id),
      throwsA(
        isA<ApiError>().having(
          (e) => e.message,
          'message',
          'Someone in this expense has left the group, so it cannot be '
              'restored.',
        ),
      ),
    );
    await expectLater(
      expenses.delete(kept.id),
      throwsA(
        isA<ApiError>().having(
          (e) => e.message,
          'message',
          'Someone in this expense has left the group, so it cannot be '
              'deleted.',
        ),
      ),
    );
    expect((await expenses.get(gone.id)).deleted, isTrue);
    expect((await groups.balances(group.id)).debts, isEmpty);
  });

  test('only members change a group\'s expenses', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id, alex = group.members[2].id;
    final e = await expenses.create(
      ExpenseInput(
        groupId: group.id,
        description: 'Taxi',
        currency: 'EUR',
        total: 2000,
        date: '2026-11-20',
        payers: [Payer(userId: sam, amount: 2000)],
        splits: [
          SplitEntry(userId: sam),
          SplitEntry(userId: alex),
        ],
      ),
    );
    // The app no longer lets the owner leave; a stored group without them
    // still keeps its expenses out of reach.
    final db = await ledger.db;
    final stored = await ledger.group(group.id, db);
    await Stores.groups.record(group.id).put(db, {
      ...stored,
      'memberIds': [sam, alex],
    });
    final notFound = isA<ApiError>().having((e) => e.code, 'code', 'NOT_FOUND');
    await expectLater(expenses.delete(e.id), throwsA(notFound));
    await expectLater(
      expenses.update(e.id, taxi(people: [sam, alex])),
      throwsA(notFound),
    );
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

  test('a monthly series keeps its day at month ends', () async {
    final expenses = LocalExpensesRepository(ledger);
    final sam = (await LocalFriendsRepository(ledger).add(name: 'Sam')).user.id;
    ExpenseInput rent(String date, int total) => ExpenseInput(
      description: 'Rent',
      currency: 'EUR',
      total: total,
      date: date,
      recurrence: Recurrence.monthly,
      payers: [Payer(userId: meId, amount: total)],
      splits: [
        const SplitEntry(userId: meId),
        SplitEntry(userId: sam),
      ],
    );
    now = DateTime.utc(2027, 3, 1, 12);
    await expenses.create(rent('2027-01-31', 1000));
    expect(await ledger.runRecurring(), 1); // Feb 28
    // Editing the copy that carries the series keeps it on the 31st.
    final february = (await expenses.list(const ExpenseQuery())).items.first;
    expect(february.date, '2027-02-28');
    await expenses.update(february.id, rent('2027-02-28', 1200));
    now = DateTime.utc(2027, 5, 1, 12);
    expect(await ledger.runRecurring(), 2); // Mar 31 and Apr 30
    final list = await expenses.list(const ExpenseQuery());
    expect(list.items.map((e) => e.date), [
      '2027-04-30',
      '2027-03-31',
      '2027-02-28',
      '2027-01-31',
    ]);
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

  test('export matches the server: decimals and former members', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id, alex = group.members[2].id;
    await expenses.create(
      ExpenseInput(
        groupId: group.id,
        description: 'Dinner',
        category: 'food.dining',
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
    now = now.add(const Duration(minutes: 1));
    await expenses.recordPayment(
      groupId: group.id,
      fromUserId: sam,
      toUserId: meId,
      amount: 3000,
      currency: 'EUR',
      date: '2026-11-20',
    );
    await groups.removeMember(group.id, sam);

    final lines = const LineSplitter().convert(
      utf8.decode(await groups.export(group.id)),
    );
    expect(lines, hasLength(3));
    expect(
      lines[0],
      'Date,Description,Category,Kind,Currency,Cost,Ada,Alex,Sam',
    );
    expect(
      lines[1],
      '2026-11-20,Dinner,food.dining,expense,EUR,90.00,60.00,-30.00,-30.00',
    );
    expect(lines[2], endsWith(',payment,EUR,30.00,-30.00,0.00,30.00'));
  });

  test('export neutralises formulas in names and descriptions', () async {
    final groups = LocalGroupsRepository(ledger);
    final group = await groups.create(
      const GroupInput(
        name: 'Flat',
        members: [MemberInput(name: '=1+1')],
      ),
    );
    final sam = group.members[1].id;
    await LocalExpensesRepository(ledger).create(
      ExpenseInput(
        groupId: group.id,
        description: '@SUM(A1)',
        currency: 'EUR',
        total: 200,
        date: '2026-11-20',
        payers: const [Payer(userId: meId, amount: 200)],
        splits: [
          const SplitEntry(userId: meId),
          SplitEntry(userId: sam),
        ],
      ),
    );
    final rows = [
      for (final line in const LineSplitter().convert(
        utf8.decode(await groups.export(group.id)),
      ))
        line.split(','),
    ];
    expect(rows[0].sublist(6), ['Ada', "'=1+1"]);
    expect(rows[1][1], "'@SUM(A1)");
    expect(rows[1].sublist(6), ['1.00', '-1.00']);
  });

  /// An expense in [group] paid by me and shared as [splits] says.
  Future<Expense> spend(
    LocalExpensesRepository expenses,
    String? group,
    String description,
    String date,
    int total, {
    String currency = 'EUR',
    String? category,
    required List<String> splits,
  }) => expenses.create(
    ExpenseInput(
      groupId: group,
      description: description,
      currency: currency,
      total: total,
      date: date,
      category: category,
      payers: [Payer(userId: meId, amount: total)],
      splits: [for (final id in splits) SplitEntry(userId: id)],
    ),
  );

  test('local stats honours the one-year window and currency', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final categories = LocalCategoriesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id;
    final other = await trip(groups);
    final pets = await categories.create(const CategoryInput(name: 'Pets'));

    // Today is 2026-11-24: the window opens on 2025-11-24.
    await spend(
      expenses,
      group.id,
      'Edge',
      '2025-11-24',
      3000,
      category: 'food.dining',
      splits: [meId, sam],
    );
    await spend(
      expenses,
      group.id,
      'Too old',
      '2025-11-23',
      5000,
      category: 'food.dining',
      splits: [meId, sam],
    );
    await spend(
      expenses,
      group.id,
      'Vet',
      '2026-10-02',
      1000,
      category: pets.id,
      splits: [meId, sam],
    );
    await spend(
      expenses,
      group.id,
      'Dollars',
      '2026-11-01',
      7000,
      currency: 'USD',
      splits: [meId, sam],
    );
    await spend(
      expenses,
      other.id,
      'Elsewhere',
      '2026-11-01',
      9000,
      splits: [meId, other.members[1].id],
    );
    await expenses.recordPayment(
      groupId: group.id,
      fromUserId: sam,
      toUserId: meId,
      amount: 500,
      currency: 'EUR',
      date: '2026-11-20',
    );
    await categories.delete(pets.id);

    final stats = await groups.stats(groupId: group.id);
    expect(stats.currency, 'EUR');
    expect(stats.totalSpent, 4000);
    expect(
      {for (final c in stats.byCategory) c.category: c.amount},
      {'food.dining': 3000, 'general': 1000},
    );
    expect(
      {for (final m in stats.byMonth) m.month: m.amount},
      {'2025-11': 3000, '2026-10': 1000},
    );
    expect(
      {for (final m in stats.byMember) m.userId: (m.paid, m.owed)},
      {meId: (4000, 2000), sam: (0, 2000)},
    );

    final dollars = await groups.stats(groupId: group.id, currency: 'USD');
    expect(dollars.totalSpent, 7000);
  });

  test('local personal stats count only what I owe', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final group = await trip(groups);
    final sam = group.members[1].id, alex = group.members[2].id;
    await spend(
      expenses,
      group.id,
      'Dinner',
      '2026-11-20',
      3000,
      splits: [meId, sam, alex],
    );
    await spend(
      expenses,
      group.id,
      'Their taxi',
      '2026-11-21',
      2000,
      splits: [sam, alex],
    );

    final stats = await groups.stats();
    expect(stats.currency, 'EUR');
    expect(stats.totalSpent, 1000);
    expect(stats.byMember, isEmpty);
  });

  test('local delete refuses unsettled and cascades', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final activity = LocalActivityRepository(ledger);
    final group = await trip(groups);
    final kept = await trip(groups);
    final sam = group.members[1].id;
    await spend(
      expenses,
      group.id,
      'Dinner',
      '2026-11-20',
      2000,
      splits: [meId, sam],
    );
    final stays = await spend(
      expenses,
      kept.id,
      'Lunch',
      '2026-11-20',
      1000,
      splits: [meId, kept.members[1].id],
    );
    final keptActivity = (await activity.list(groupId: kept.id)).total;

    await expectLater(
      groups.delete(group.id),
      throwsA(
        isA<ApiError>()
            .having((e) => e.status, 'status', 409)
            .having(
              (e) => e.message,
              'message',
              'Settle up before deleting this group.',
            ),
      ),
    );
    expect((await groups.get(group.id)).id, group.id);

    await expenses.recordPayment(
      groupId: group.id,
      fromUserId: sam,
      toUserId: meId,
      amount: 1000,
      currency: 'EUR',
    );
    await groups.delete(group.id);

    expect((await groups.list()).map((g) => g.id), [kept.id]);
    await expectLater(groups.get(group.id), throwsA(isA<ApiError>()));
    expect((await activity.list(groupId: group.id)).total, 0);
    expect((await activity.list(groupId: kept.id)).total, keptActivity);
    final left = await expenses.list(const ExpenseQuery());
    expect(left.items.map((e) => e.id), [stays.id]);
  });
}
