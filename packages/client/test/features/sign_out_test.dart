import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/sync/outbox.dart';

import '../helpers.dart';

const _server = 'http://splitspends.test';
const _owner = (server: _server, user: 'u1');

/// Answers every read with an empty or minimal body; `/me` with [me], or
/// 401 when [me] is null.
http.Client _api({Map<String, Object?>? me}) => MockClient((request) async {
  final path = request.url.path;
  if (path.endsWith('/me')) {
    return me == null
        ? http.Response(
            jsonEncode({'error': 'Please sign in.', 'code': 'UNAUTHORIZED'}),
            401,
          )
        : http.Response(jsonEncode({'data': me}), 200);
  }
  if (request.method != 'GET') return http.Response('', 204);
  final Object body = path.endsWith('/config')
      ? {
          'data': {
            'appName': 'SplitSpends',
            'currencies': <Object>[],
            'categories': <Object>[],
          },
        }
      : path.endsWith('/me/balances')
      ? {
          'data': {
            'balances': <Object>[],
            'owed': <Object>[],
            'owing': <Object>[],
          },
        }
      : {'items': <Object>[], 'total': 0};
  return http.Response(jsonEncode(body), 200);
});

Future<ProviderContainer> _start(
  WidgetTester tester,
  MemoryLocalStore store,
  http.Client api,
) async {
  tester.view.physicalSize = const Size(420, 2000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        localStoreProvider.overrideWithValue(store),
        memoryDatabase(),
        httpClientProvider.overrideWithValue(api),
      ],
      child: const SplitSpendsApp(),
    ),
  );
  await settle(tester);
  return ProviderScope.containerOf(tester.element(find.byType(SplitSpendsApp)));
}

Future<Outbox> _outbox(WidgetTester tester, ProviderContainer container) async {
  final db = await pumpFuture(tester, container.read(databaseProvider.future));
  return Outbox(() async => db, owner: _owner);
}

/// Pumps frames until [finder] finds one widget, for screens that wait on
/// the database.
Future<void> _pumpUntil(WidgetTester tester, Finder finder) async {
  for (var i = 0; i < 100 && finder.evaluate().isEmpty; i++) {
    await tester.pump(const Duration(milliseconds: 20));
  }
  await settle(tester);
  expect(finder, findsOneWidget);
}

void main() {
  testWidgets('signing out with unsynced changes asks first', (tester) async {
    final store = MemoryLocalStore(
      serverUrl: _server,
      token: 'tok',
      appMode: AppMode.connected.name,
    );
    // The server is unreachable for writes, so the change stays queued.
    final container = await _start(
      tester,
      store,
      _api(me: {'id': 'u1', 'name': 'Ada', 'defaultCurrency': 'EUR'}),
    );
    final outbox = await _outbox(tester, container);
    await pumpFuture(
      tester,
      outbox.add('POST', '/expenses', body: {'id': 'e1'}, label: 'Taxi'),
    );

    await _pumpUntil(tester, find.text('1 change waiting to sync'));
    GoRouter.of(tester.element(find.byType(Scaffold).first)).go(Routes.account);
    await settle(tester);
    await tester.tap(find.text('Sign out'));
    await _pumpUntil(tester, find.text('Sign out anyway?'));

    expect(find.textContaining('1 change has not reached'), findsOneWidget);
    await tester.tap(find.text('Stay signed in'));
    await settle(tester);
    expect(store.token, 'tok');
    expect(await pumpFuture(tester, outbox.pending()), hasLength(1));

    await tester.tap(find.text('Sign out'));
    await _pumpUntil(tester, find.widgetWithText(FilledButton, 'Sign out'));
    await tester.tap(find.widgetWithText(FilledButton, 'Sign out'));
    await settle(tester);
    expect(store.token, isNull);
    expect(await pumpFuture(tester, outbox.pending()), isEmpty);
    expect(find.text('Use on this device'), findsOneWidget);
  });

  testWidgets('an ended session shows the changes waiting for it', (
    tester,
  ) async {
    final store = MemoryLocalStore(
      serverUrl: _server,
      token: 'expired',
      appMode: AppMode.connected.name,
    );
    final container = await _start(tester, store, _api());
    final outbox = await _outbox(tester, container);
    await pumpFuture(
      tester,
      outbox.add('POST', '/expenses', body: {'id': 'e1'}, label: 'Taxi'),
    );
    await settle(tester);

    expect(find.text('Use on this device'), findsOneWidget);
    expect(
      find.textContaining('1 change made on this device has not synced'),
      findsOneWidget,
    );

    await tester.tap(find.text('Discard'));
    await settle(tester);
    expect(find.text('Discard unsynced changes?'), findsOneWidget);
    await tester.tap(find.widgetWithText(FilledButton, 'Discard'));
    await settle(tester);
    expect(find.byKey(const Key('waiting-changes')), findsNothing);
    expect(await pumpFuture(tester, outbox.pending()), isEmpty);
  });
}
