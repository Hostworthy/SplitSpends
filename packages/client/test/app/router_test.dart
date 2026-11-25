import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';

import '../helpers.dart';

const _local = Session(
  mode: AppMode.local,
  me: Me(id: 'ada', name: 'Ada'),
);

/// A session whose loads finish when the test completes them, in order.
class _Session extends SessionController {
  _Session(this.loads);

  final List<Completer<Session>> loads;
  var _builds = 0;

  @override
  Future<Session> build() => loads[_builds++].future;
}

void main() {
  late List<Completer<Session>> loads;

  Future<GoRouter> pump(WidgetTester tester) async {
    // Made inside the test so they complete under the tester's fake clock.
    loads = [Completer(), Completer()];
    tester.view.physicalSize = const Size(420, 1000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(MemoryLocalStore()),
          memoryDatabase(),
          sessionProvider.overrideWith(() => _Session(loads)),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    final container = ProviderScope.containerOf(
      tester.element(find.byType(SplitSpendsApp)),
    );
    return container.read(routerProvider);
  }

  String location(GoRouter router) =>
      router.routerDelegate.currentConfiguration.uri.toString();

  testWidgets('a deep link waits on the splash and opens once ready', (
    tester,
  ) async {
    final router = await pump(tester);
    router.go(Routes.activity);
    await tester.pump();
    expect(location(router), '${Routes.splash}?from=%2Factivity');

    loads.first.complete(_local);
    await settle(tester);
    expect(location(router), Routes.activity);
  });

  testWidgets('a session that fails to load stays on the splash with a way '
      'out, and Try again continues to the deep link', (tester) async {
    final router = await pump(tester);
    router.go(Routes.activity);
    await tester.pump();

    loads.first.completeError(StateError('The database is locked.'));
    await settle(tester);
    expect(location(router), startsWith(Routes.splash));
    expect(find.text('Could not open your data'), findsOneWidget);

    await tester.tap(find.text('Try again'));
    await tester.pump();
    loads[1].complete(_local);
    await settle(tester);
    expect(location(router), Routes.activity);
  });

  testWidgets('without a chosen mode only onboarding and invites open', (
    tester,
  ) async {
    final router = await pump(tester);
    loads.first.complete(const Session());
    await settle(tester);
    expect(location(router), Routes.welcome);

    router.go(Routes.groups);
    await settle(tester);
    expect(location(router), Routes.welcome);

    router.go(Routes.login);
    await settle(tester);
    expect(location(router), Routes.login);
  });

  testWidgets('on this device, sign-in stays open and welcome goes home', (
    tester,
  ) async {
    final router = await pump(tester);
    loads.first.complete(_local);
    await settle(tester);
    expect(location(router), Routes.home);

    router.go(Routes.login);
    await settle(tester);
    expect(location(router), Routes.login);

    router.go(Routes.welcome);
    await settle(tester);
    expect(location(router), Routes.home);

    router.go(Routes.splash);
    await settle(tester);
    expect(location(router), Routes.home);
  });
}
