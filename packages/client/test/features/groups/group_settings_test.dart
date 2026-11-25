import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/group_settings_screen.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

const _me = Me(id: 'ada', name: 'Ada');

final _group = Group.fromJson({
  'id': 'g1',
  'name': 'Trip',
  'kind': 'trip',
  'defaultCurrency': 'EUR',
  'simplifyDebts': true,
  'archived': false,
  'members': [
    {'id': 'ada', 'name': 'Ada'},
    {'id': 'sam', 'name': 'Sam'},
  ],
  'balances': <Object>[],
});

Future<void> _pump(WidgetTester tester, AppMode mode) async {
  tester.view.physicalSize = const Size(420, 2000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        appModeProvider.overrideWithValue(mode),
        meProvider.overrideWithValue(_me),
        groupProvider('g1').overrideWith((ref) async => _group),
      ],
      child: const MaterialApp(home: GroupSettingsScreen(groupId: 'g1')),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('on-device groups offer archive and delete, not leave', (
    tester,
  ) async {
    await _pump(tester, AppMode.local);
    expect(find.text('Delete group'), findsOneWidget);
    expect(find.text('Leave group'), findsNothing);
  });

  testWidgets('connected members can leave a group', (tester) async {
    await _pump(tester, AppMode.connected);
    expect(find.text('Leave group'), findsOneWidget);
  });
}
