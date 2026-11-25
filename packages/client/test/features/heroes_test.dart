import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/features/friends/data/providers.dart';
import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/groups/domain/group.dart';

import '../helpers.dart';

/// A group or friend can show up as a hero in several places under one
/// route: the list and the detail pane side by side on a wide screen, or the
/// dashboard tab kept alive behind the groups tab. Opening another screen
/// from there must not trip over two heroes with one tag.
void main() {
  const wide = Size(1200, 800);
  const phone = Size(420, 900);

  Future<ProviderContainer> start(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
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
    return ProviderScope.containerOf(
      tester.element(find.byType(SplitSpendsApp)),
    );
  }

  for (final (action, size, layout) in [
    ('Group settings', wide, 'two-pane'),
    ('Settle up', wide, 'two-pane'),
    ('Settle up', phone, 'phone'),
  ]) {
    testWidgets('$action opens from the $layout group view', (tester) async {
      final container = await start(tester, size);
      final group = await pumpFuture(
        tester,
        container
            .read(groupsRepositoryProvider)
            .create(
              const GroupInput(
                name: 'Cabin',
                members: [MemberInput(name: 'Sam')],
              ),
            ),
      );
      refreshLedgerIn(container);
      container.read(routerProvider).go(Routes.group(group.id));
      await settle(tester);
      expect(find.text('Cabin'), findsWidgets);

      await tester.tap(find.byTooltip(action));
      await settle(tester);
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets('settle up opens from the two-pane friends view', (tester) async {
    final container = await start(tester, wide);
    final friend = await pumpFuture(
      tester,
      container.read(friendsRepositoryProvider).add(name: 'Sam'),
    );
    refreshLedgerIn(container);
    container.read(routerProvider).go(Routes.friend(friend.user.id));
    await settle(tester);

    await tester.tap(find.text('Settle up'));
    await settle(tester);
    expect(tester.takeException(), isNull);
  });
}
