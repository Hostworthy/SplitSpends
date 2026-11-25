import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';

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
}
