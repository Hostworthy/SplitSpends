import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
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
  'color': '#5E35B1',
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

/// Holds every update until the test releases it, and counts list calls so
/// a test can see whether the groups list was refreshed.
class _SlowRepo extends Fake implements GroupsRepository {
  final pending = <Completer<Group>>[];
  var lists = 0;

  @override
  Future<List<Group>> list({bool archived = false}) async {
    if (!archived) lists++;
    return [_group];
  }

  @override
  Future<Group> get(String id) async => _group;

  @override
  Future<Group> update(String id, GroupInput input) {
    final done = Completer<Group>();
    pending.add(done);
    return done.future;
  }
}

/// Opens group settings over a screen that shows the groups list.
Future<_SlowRepo> _pushSettings(WidgetTester tester) async {
  tester.view.physicalSize = const Size(420, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  final repo = _SlowRepo();
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        appModeProvider.overrideWithValue(AppMode.local),
        meProvider.overrideWithValue(_me),
        groupsRepositoryProvider.overrideWithValue(repo),
      ],
      child: MaterialApp(
        home: Consumer(
          builder: (context, ref, _) {
            ref.watch(groupsProvider);
            return TextButton(
              onPressed: () => Navigator.push(
                context,
                MaterialPageRoute<void>(
                  builder: (_) => const GroupSettingsScreen(groupId: 'g1'),
                ),
              ),
              child: const Text('Open'),
            );
          },
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
  await tester.tap(find.text('Open'));
  await tester.pumpAndSettle();
  return repo;
}

void main() {
  for (final (name, act) in [
    (
      'rename',
      (WidgetTester tester) async {
        await tester.enterText(find.byType(TextField).first, 'Holiday');
        await tester.tap(find.byIcon(Icons.check_rounded));
      },
    ),
    (
      'default split',
      (WidgetTester tester) async {
        final section = find.byKey(const Key('default-split'));
        await tester.ensureVisible(section);
        await tester.tap(
          find.descendant(of: section, matching: find.text('Equally')),
        );
        await tester.pumpAndSettle();
        await tester.tap(find.byKey(const Key('save-default-split')));
      },
    ),
  ]) {
    testWidgets('a $name saved after leaving settings refreshes the groups', (
      tester,
    ) async {
      final repo = await _pushSettings(tester);
      expect(repo.lists, 1);
      await act(tester);
      await tester.pump();
      expect(repo.pending, hasLength(1));

      tester.state<NavigatorState>(find.byType(Navigator)).pop();
      await tester.pumpAndSettle();
      expect(find.byType(GroupSettingsScreen), findsNothing);

      repo.pending.single.complete(_group);
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      expect(repo.lists, 2);
    });
  }

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

  testWidgets('the group color swatches are named and show the chosen one', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    await _pump(tester, AppMode.local);
    expect(
      tester.getSemantics(find.bySemanticsLabel('Deep purple color')),
      isSemantics(isButton: true, isSelected: true),
    );
    expect(
      tester.getSemantics(find.bySemanticsLabel('Teal color')),
      isSemantics(isButton: true, isSelected: false),
    );
    handle.dispose();
  });
}
