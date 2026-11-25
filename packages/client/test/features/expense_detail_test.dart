import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/expenses/data/providers.dart';
import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';

import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/friends/presentation/people.dart';

import '../helpers.dart';

/// Starts the app on the device as Ada.
Future<ProviderContainer> _start(
  WidgetTester tester, {
  List<Override> overrides = const [],
}) async {
  tester.view.physicalSize = const Size(420, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        localStoreProvider.overrideWithValue(MemoryLocalStore()),
        memoryDatabase(),
        ...overrides,
      ],
      child: const SplitSpendsApp(),
    ),
  );
  await settle(tester);
  await tester.tap(find.text('Start'));
  await settle(tester);
  await tester.enterText(find.byKey(const Key('local-name')), 'Ada');
  await tester.tap(find.byKey(const Key('local-start')));
  await settle(tester);
  return ProviderScope.containerOf(tester.element(find.byType(SplitSpendsApp)));
}

/// Counts restores and list fetches.
class _Counting extends LocalExpensesRepository {
  _Counting(super.ledger);

  var restores = 0;
  var lists = 0;

  @override
  Future<Expense> restore(String id) {
    restores++;
    return super.restore(id);
  }

  @override
  Future<PageResult<Expense>> list(
    ExpenseQuery query, {
    int limit = 50,
    int offset = 0,
  }) {
    lists++;
    return super.list(query, limit: limit, offset: offset);
  }
}

void main() {
  testWidgets('undo after delete restores and refreshes', (tester) async {
    late _Counting repo;
    final container = await _start(
      tester,
      overrides: [
        expensesRepositoryProvider.overrideWith(
          (ref) => repo = _Counting(ref.watch(localLedgerProvider)),
        ),
      ],
    );
    final me = container.read(meProvider)!;
    container.read(expensesRepositoryProvider);
    final flat = await pumpFuture(
      tester,
      container
          .read(groupsRepositoryProvider)
          .create(
            const GroupInput(
              name: 'Flat',
              members: [MemberInput(name: 'Ben')],
            ),
          ),
    );
    final taxi = await pumpFuture(
      tester,
      repo.create(
        ExpenseInput(
          groupId: flat.id,
          description: 'Taxi',
          currency: 'EUR',
          total: 1200,
          date: '2026-11-20',
          payers: [Payer(userId: me.id, amount: 1200)],
          splits: [for (final m in flat.members) SplitEntry(userId: m.id)],
        ),
      ),
    );
    final feed = container.listen(
      expensesFeedProvider(ExpenseQuery(groupId: flat.id)),
      (_, _) {},
    );
    addTearDown(feed.close);
    unawaited(container.read(routerProvider).push(Routes.expense(taxi.id)));
    await settle(tester);

    await tester.tap(find.byTooltip('Delete'));
    await settle(tester);
    final fetched = repo.lists;
    await tester.tap(find.text('Undo'));
    await settle(tester);

    expect(repo.restores, 1);
    expect(find.text('Could not restore'), findsNothing);
    expect(repo.lists, greaterThan(fetched));
    final restored = await pumpFuture(
      tester,
      container.read(
        expensesFeedProvider(ExpenseQuery(groupId: flat.id)).future,
      ),
    );
    expect(restored.items.map((e) => e.id), [taxi.id]);
  });

  testWidgets('an expense split by an unknown method cannot be edited', (
    tester,
  ) async {
    final container = await _start(
      tester,
      overrides: [
        expenseProvider.overrideWith(
          (ref, id) async => Expense.fromJson({
            'id': id,
            'description': 'Rent',
            'currency': 'EUR',
            'total': 9000,
            'date': '2026-11-20',
            'splitMethod': 'ratio',
            'recurrence': 'daily',
            'shares': [
              {'userId': 'u1', 'paid': 9000, 'owed': 6000},
              {'userId': 'u2', 'paid': 0, 'owed': 3000},
            ],
          }),
        ),
      ],
    );
    unawaited(container.read(routerProvider).push(Routes.expense('e1')));
    await settle(tester);

    expect(find.text('Split by ratio'), findsOneWidget);
    expect(find.text('Repeats: daily'), findsOneWidget);
    final edit = tester.widget<IconButton>(
      find.widgetWithIcon(IconButton, Icons.edit_rounded),
    );
    expect(edit.onPressed, isNull);
    expect(edit.tooltip, 'Update the app to edit this expense');

    unawaited(container.read(routerProvider).push(Routes.editExpense('e1')));
    await settle(tester);
    expect(find.text('Update the app to edit this expense'), findsOneWidget);
    expect(find.text('Save'), findsNothing);
  });

  for (final (name, edited, want) in [
    ('a save right after adding is not an edit', 3, 'Added by you Jan 10'),
    (
      'a later edit names who made it',
      600,
      'Added by you Jan 10 · edited by Ben Jan 10',
    ),
  ]) {
    testWidgets(name, (tester) async {
      final created = DateTime.utc(2026, 1, 10, 12);
      final container = await _start(
        tester,
        overrides: [
          expenseProvider.overrideWith(
            (ref, id) async => Expense.fromJson({
              'id': id,
              'description': 'Rent',
              'currency': 'EUR',
              'total': 9000,
              'date': '2026-01-10',
              'splitMethod': 'equal',
              'createdBy': ref.read(meProvider)!.id,
              'updatedBy': 'ben',
              'createdAt': created.toIso8601String(),
              'updatedAt': created
                  .add(Duration(seconds: edited))
                  .toIso8601String(),
              'shares': const <Object>[],
            }),
          ),
          peopleProvider.overrideWithValue({
            'ben': const User(id: 'ben', name: 'Ben Okafor'),
          }),
        ],
      );
      unawaited(container.read(routerProvider).push(Routes.expense('e1')));
      await settle(tester);

      expect(find.text(want), findsOneWidget);
    });
  }
}
