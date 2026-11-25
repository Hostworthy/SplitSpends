import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/theme/theme.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/dashboard/presentation/dashboard_screen.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/presentation/groups_screen.dart';

import '../../helpers.dart';

const _failure = ApiError(status: 500, message: 'The server had a problem.');

/// Lists groups fine, or fails for the lists named in [failing].
class _Groups extends Fake implements GroupsRepository {
  _Groups({required this.failing});

  final Set<bool> failing;
  final calls = <bool>[];

  @override
  Future<List<Group>> list({bool archived = false}) async {
    calls.add(archived);
    if (failing.contains(archived)) throw _failure;
    return const [Group(id: 'g1', name: 'Flat')];
  }
}

class _Local extends SessionController {
  @override
  Future<Session> build() async => const Session(
    mode: AppMode.local,
    me: Me(id: 'ada', name: 'Ada'),
  );
}

Future<void> _pump(WidgetTester tester, _Groups groups, Widget home) async {
  tester.view.physicalSize = const Size(420, 1600);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        memoryDatabase(),
        sessionProvider.overrideWith(_Local.new),
        groupsRepositoryProvider.overrideWithValue(groups),
      ],
      child: MaterialApp(theme: AppTheme.light(), home: home),
    ),
  );
  await settle(tester);
}

Finder _retry(String key) =>
    find.descendant(of: find.byKey(Key(key)), matching: find.text('Try again'));

void main() {
  testWidgets('the dashboard says why groups did not load and retries', (
    tester,
  ) async {
    final groups = _Groups(failing: {false});
    await _pump(tester, groups, const DashboardScreen());

    final retry = _retry('groups-error');
    expect(find.text(_failure.message), findsOneWidget);
    expect(retry, findsOneWidget);
    await tester.tap(retry);
    await settle(tester);
    expect(groups.calls, [false, false]);
  });

  testWidgets('archived groups show an error instead of spinning', (
    tester,
  ) async {
    final groups = _Groups(failing: {true});
    await _pump(tester, groups, const GroupsScreen());

    await tester.tap(find.text('Show archived groups'));
    await settle(tester);
    expect(find.byType(LinearProgressIndicator), findsNothing);
    expect(find.text(_failure.message), findsOneWidget);
    await tester.tap(_retry('archived-error'));
    await settle(tester);
    expect(groups.calls.where((archived) => archived), hasLength(2));
  });
}
