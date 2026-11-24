import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/features/groups/domain/group.dart';

import '../helpers.dart';

/// On-device mode end to end: set up without an account, create people and
/// a group, add an expense through the editor, and see the right balances.
void main() {
  testWidgets('local mode: people, group, expense, balance', (tester) async {
    tester.view.physicalSize = const Size(420, 1000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    final store = MemoryLocalStore();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(store),
          memoryDatabase(),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await settle(tester);

    // Welcome → "Use on this device" → name.
    await tester.tap(find.text('Start'));
    await settle(tester);
    await tester.enterText(find.byKey(const Key('local-name')), 'Ada');
    await tester.tap(find.byKey(const Key('local-start')));
    await settle(tester);
    expect(store.appMode, 'local');

    // A group with two people added by name (through the repository the
    // UI uses).
    final container = ProviderScope.containerOf(
      tester.element(find.byType(SplitSpendsApp)),
    );
    final group = await pumpFuture(
      tester,
      container
          .read(groupsRepositoryProvider)
          .create(
            const GroupInput(
              name: 'Cabin',
              members: [
                MemberInput(name: 'Sam'),
                MemberInput(name: 'Kim'),
              ],
            ),
          ),
    );

    // Add an expense through the editor: \$90 paid by Ada, split equally.
    final router = container.read(routerProvider);
    router.push('/expenses/new?groupId=${group.id}');
    await settle(tester);
    await tester.enterText(
      find.byKey(const Key('expense-description')),
      'Groceries',
    );
    await tester.enterText(
      find.descendant(
        of: find.byType(AmountField).first,
        matching: find.byType(TextField),
      ),
      '90',
    );
    await settle(tester);
    expect(find.text('Ready to save'), findsOneWidget);
    await tester.tap(find.text('Save'));
    await settle(tester);

    // Dashboard: Ada is owed \$60 (Sam and Kim owe \$30 each).
    router.go('/');
    await settle(tester);
    expect(find.text('You are owed'), findsOneWidget);
    expect(find.text(r'$60.00'), findsWidgets);

    // The group's balances tab shows who owes whom.
    router.go('/groups/${group.id}');
    await settle(tester);
    await tester.tap(find.text('Balances'));
    await settle(tester);
    expect(find.textContaining('owes'), findsWidgets);
    expect(find.text(r'$30.00'), findsWidgets);
  });
}
