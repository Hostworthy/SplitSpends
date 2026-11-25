import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/friends/data/local_friends_repository.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';

/// Counts reads of the stores that grow with use.
class _CountingLedger extends LocalLedger {
  _CountingLedger(super.open, {super.clock});

  var expenseReads = 0;
  var directoryReads = 0;

  void reset() => expenseReads = directoryReads = 0;

  @override
  Future<List<JsonMap>> expenses([DatabaseClient? client]) {
    expenseReads++;
    return super.expenses(client);
  }

  @override
  Future<Map<String, JsonMap>> directory([DatabaseClient? client]) {
    directoryReads++;
    return super.directory(client);
  }
}

void main() {
  const meId = '00000000-0000-4000-8000-00000000000a';
  const me = Me(id: meId, name: 'Ada', defaultCurrency: 'EUR');
  late _CountingLedger ledger;

  setUp(() async {
    final db = await databaseFactoryMemory.openDatabase(
      'test-${DateTime.now().microsecondsSinceEpoch}',
    );
    await Stores.meta.record('profile').put(db, me.toJson());
    ledger = _CountingLedger(
      () async => db,
      clock: () => DateTime.utc(2026, 11, 24, 12),
    );
  });

  test('listing groups reads expenses and people once', () async {
    final groups = LocalGroupsRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    for (final name in ['Flat', 'Trip', 'Office']) {
      final group = await groups.create(
        GroupInput(
          name: name,
          members: const [MemberInput(name: 'Sam')],
        ),
      );
      final sam = group.members[1].id;
      await expenses.create(
        ExpenseInput(
          groupId: group.id,
          description: 'Dinner',
          currency: 'EUR',
          total: 3000,
          date: '2026-11-20',
          payers: const [Payer(userId: meId, amount: 3000)],
          splits: [
            const SplitEntry(userId: meId),
            SplitEntry(userId: sam),
          ],
        ),
      );
    }

    ledger.reset();
    final listed = await groups.list();
    expect(ledger.expenseReads, 1);
    expect(ledger.directoryReads, 1);
    expect(listed, hasLength(3));
    for (final g in listed) {
      expect(g.balances.single.amount, 1500, reason: g.name);
      expect(g.members.map((m) => m.displayName), ['Ada', 'Sam']);
    }
  });

  test('listing friends reads people once', () async {
    final friends = LocalFriendsRepository(ledger);
    for (final name in ['Sam', 'Kim', 'Lee']) {
      await friends.add(name: name);
    }

    ledger.reset();
    final listed = await friends.list();
    expect(ledger.directoryReads, 1);
    expect(listed.map((f) => f.user.displayName), ['Kim', 'Lee', 'Sam']);
  });
}
