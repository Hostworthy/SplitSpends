import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';

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

void main() {
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
}
