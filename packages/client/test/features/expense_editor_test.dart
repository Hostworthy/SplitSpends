import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

import '../helpers.dart';

/// Fails the first create after it may have reached the server.
class _Flaky extends LocalExpensesRepository {
  _Flaky(super.ledger);

  final ids = <String?>[];

  @override
  Future<Expense> create(ExpenseInput input) async {
    ids.add(input.id);
    if (ids.length == 1) {
      throw const ApiError.network('Cannot reach the server.');
    }
    return super.create(input);
  }
}

/// Holds every create until [release] completes, and counts list fetches.
class _Slow extends LocalExpensesRepository {
  _Slow(super.ledger);

  final release = Completer<void>();
  var lists = 0;

  @override
  Future<Expense> create(ExpenseInput input) async {
    await release.future;
    return super.create(input);
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

/// Starts the app on the device as Ada, with a group "Flat" she shares
/// with Ben.
Future<(ProviderContainer, Group)> _start(
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

  final container = ProviderScope.containerOf(
    tester.element(find.byType(SplitSpendsApp)),
  );
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
  return (container, flat);
}

void main() {
  testWidgets('saving again after a failure keeps the same expense id', (
    tester,
  ) async {
    late _Flaky flaky;
    final (container, flat) = await _start(
      tester,
      overrides: [
        expensesRepositoryProvider.overrideWith(
          (ref) => flaky = _Flaky(ref.watch(localLedgerProvider)),
        ),
      ],
    );
    unawaited(
      container.read(routerProvider).push('/expenses/new?groupId=${flat.id}'),
    );
    await settle(tester);
    await tester.enterText(
      find.byKey(const Key('expense-description')),
      'Taxi',
    );
    await tester.enterText(
      find.descendant(
        of: find.byType(AmountField).first,
        matching: find.byType(TextField),
      ),
      '12',
    );
    await settle(tester);

    await tester.tap(find.text('Save'));
    await settle(tester);
    expect(flaky.ids, hasLength(1));
    await tester.tap(find.text('Save'));
    await settle(tester);

    expect(flaky.ids, hasLength(2));
    expect(flaky.ids.first, isNotNull);
    expect(flaky.ids.last, flaky.ids.first);
    final list = await pumpFuture(
      tester,
      container
          .read(expensesRepositoryProvider)
          .list(ExpenseQuery(groupId: flat.id)),
    );
    expect(list.items.single.id, flaky.ids.first);
  });

  testWidgets('editing an expense does not offer a group change', (
    tester,
  ) async {
    final (container, flat) = await _start(tester);
    final me = container.read(meProvider)!;
    final taxi = await pumpFuture(
      tester,
      container
          .read(expensesRepositoryProvider)
          .create(
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
    unawaited(container.read(routerProvider).push('/expenses/${taxi.id}/edit'));
    await settle(tester);

    final chip = tester.widget<ActionChip>(
      find.widgetWithIcon(ActionChip, Icons.groups_rounded),
    );
    expect(chip.onPressed, isNull);
    expect(chip.tooltip, 'The group of an expense cannot change');
  });

  for (final date in ['1999-12-31', '2099-01-01']) {
    testWidgets('editing an expense dated $date opens the date picker', (
      tester,
    ) async {
      final (container, flat) = await _start(tester);
      final me = container.read(meProvider)!;
      final taxi = await pumpFuture(
        tester,
        container
            .read(expensesRepositoryProvider)
            .create(
              ExpenseInput(
                groupId: flat.id,
                description: 'Taxi',
                currency: 'EUR',
                total: 1200,
                date: '2026-11-20',
                payers: [Payer(userId: me.id, amount: 1200)],
                splits: [
                  for (final m in flat.members) SplitEntry(userId: m.id),
                ],
              ),
            ),
      );
      // Stored before the date rules existed, or by another client.
      final ledger = container.read(localLedgerProvider);
      await pumpFuture(
        tester,
        ledger.db.then(
          (db) => Stores.expenses.record(taxi.id).update(db, {'date': date}),
        ),
      );
      unawaited(
        container.read(routerProvider).push('/expenses/${taxi.id}/edit'),
      );
      await settle(tester);

      await tester.tap(find.widgetWithIcon(ActionChip, Icons.event_rounded));
      await settle(tester);
      expect(tester.takeException(), isNull);
      expect(find.byType(DatePickerDialog), findsOneWidget);
    });
  }

  testWidgets('picking a group in another currency does not keep the amount', (
    tester,
  ) async {
    final (container, _) = await _start(tester);
    await pumpFuture(
      tester,
      container
          .read(groupsRepositoryProvider)
          .create(
            const GroupInput(
              name: 'Tokyo',
              defaultCurrency: 'JPY',
              members: [MemberInput(name: 'Kei')],
            ),
          ),
    );
    container.invalidate(groupsProvider);
    unawaited(container.read(routerProvider).push('/expenses/new'));
    await settle(tester);
    await tester.enterText(
      find.byKey(const Key('expense-description')),
      'Sushi',
    );
    final total = find.descendant(
      of: find.byType(AmountField).first,
      matching: find.byType(TextField),
    );
    await tester.enterText(total, '12.50');
    await settle(tester);

    await tester.tap(find.widgetWithText(ActionChip, 'No group'));
    await settle(tester);
    await tester.tap(find.text('Tokyo'));
    await settle(tester);

    expect(
      find.text('Amounts are now in JPY. Please enter them again.'),
      findsOneWidget,
    );
    expect(find.text('Enter an amount'), findsOneWidget);
    await tester.enterText(total, '1250');
    await settle(tester);
    expect(find.text('Enter an amount'), findsNothing);
  });

  testWidgets('closing the editor during save still refreshes the ledger', (
    tester,
  ) async {
    late _Slow slow;
    final (container, flat) = await _start(
      tester,
      overrides: [
        expensesRepositoryProvider.overrideWith(
          (ref) => slow = _Slow(ref.watch(localLedgerProvider)),
        ),
      ],
    );
    final query = ExpenseQuery(groupId: flat.id);
    final feed = container.listen(expensesFeedProvider(query), (_, _) {});
    addTearDown(feed.close);
    unawaited(
      container.read(routerProvider).push('/expenses/new?groupId=${flat.id}'),
    );
    await settle(tester);
    await tester.enterText(
      find.byKey(const Key('expense-description')),
      'Taxi',
    );
    await tester.enterText(
      find.descendant(
        of: find.byType(AmountField).first,
        matching: find.byType(TextField),
      ),
      '12',
    );
    await settle(tester);
    await tester.tap(find.text('Save'));
    await tester.pump();

    container.read(routerProvider).pop();
    await settle(tester);
    expect(find.text('Save'), findsNothing);
    final fetched = slow.lists;
    slow.release.complete();
    await settle(tester);

    expect(tester.takeException(), isNull);
    expect(slow.lists, greaterThan(fetched));
    final page = await pumpFuture(
      tester,
      container.read(expensesFeedProvider(query).future),
    );
    expect(page.items.single.description, 'Taxi');
  });
}
