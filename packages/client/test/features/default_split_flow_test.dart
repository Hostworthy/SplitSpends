import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/presentation/editor/split_section.dart';
import 'package:splitspends/features/groups/domain/group.dart';

import '../helpers.dart';

/// On-device mode: set rent to 60/40 in group settings, and a new expense in
/// that group starts with that split.
void main() {
  testWidgets('default split from group settings to the editor', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 2400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(MemoryLocalStore()),
          memoryDatabase(),
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
    final router = container.read(routerProvider);
    router.push('/groups/${flat.id}/settings');
    await settle(tester);

    final section = find.byKey(const Key('default-split'));
    await tester.ensureVisible(section);
    await tester.tap(
      find.descendant(of: section, matching: find.text('Percent')),
    );
    await settle(tester);
    final fields = find.descendant(
      of: section,
      matching: find.descendant(
        of: find.byType(DecimalField),
        matching: find.byType(TextField),
      ),
    );
    expect(fields, findsNWidgets(2));
    await tester.enterText(fields.at(0), '70');
    await settle(tester);
    expect(find.text('Percentages must add up to 100'), findsOneWidget);
    await tester.enterText(fields.at(0), '60');
    await tester.enterText(fields.at(1), '40');
    await settle(tester);
    expect(find.text('100% assigned'), findsOneWidget);
    await tester.tap(find.byKey(const Key('save-default-split')));
    await settle(tester);

    final saved = await pumpFuture(
      tester,
      container.read(groupsRepositoryProvider).get(flat.id),
    );
    expect(saved.defaultSplit?.method, SplitMethod.percent);

    // A new expense in the flat starts at 60/40.
    router.push('/expenses/new?groupId=${flat.id}');
    await settle(tester);
    await tester.enterText(
      find.byKey(const Key('expense-description')),
      'Rent',
    );
    await tester.enterText(
      find.descendant(
        of: find.byType(AmountField).first,
        matching: find.byType(TextField),
      ),
      '1000',
    );
    await settle(tester);
    expect(find.text(r'$600.00'), findsOneWidget);
    expect(find.text(r'$400.00'), findsOneWidget);
    await tester.tap(find.text('Save'));
    await settle(tester);

    final list = await pumpFuture(
      tester,
      container
          .read(expensesRepositoryProvider)
          .list(ExpenseQuery(groupId: flat.id)),
    );
    final rent = list.items.single;
    expect(rent.splitMethod, SplitMethod.percent);
    expect({for (final s in rent.shares) s.value}, {'60', '40'});
  });
}
