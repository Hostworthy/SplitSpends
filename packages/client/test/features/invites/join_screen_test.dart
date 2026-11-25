import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';

import '../../helpers.dart';

const _token = 'k3V9xQ2mN8pL4rT7wY1zA6bC0dE5fG8hJ2kM4nP6qR8';
const _home = 'https://split.example.com';
const _other = 'https://other.example.com';

/// Opens `/join/<token>?server=[server]` for a visitor without a session
/// whose app talks to [_home], and returns the hosts the app asked for a
/// preview.
Future<List<String>> _open(
  WidgetTester tester,
  MemoryLocalStore store,
  String server, {
  List<Override> overrides = const [],
}) async {
  tester.view.physicalSize = const Size(420, 900);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  final previews = <String>[];
  final api = MockClient((request) async {
    Object body = {'items': <Object>[], 'total': 0};
    if (request.url.path.endsWith('/invites/$_token')) {
      previews.add(request.url.origin);
      body = {
        'data': {'groupName': 'Flat', 'memberCount': 2, 'invitedBy': 'Ada'},
      };
    }
    return http.Response(
      jsonEncode(body),
      200,
      headers: {'content-type': 'application/json'},
    );
  });
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        localStoreProvider.overrideWithValue(store),
        memoryDatabase(),
        httpClientProvider.overrideWithValue(api),
        ...overrides,
      ],
      child: const SplitSpendsApp(),
    ),
  );
  await settle(tester);
  final container = ProviderScope.containerOf(
    tester.element(find.byType(SplitSpendsApp)),
  );
  container
      .read(routerProvider)
      .go('/join/$_token?server=${Uri.encodeQueryComponent(server)}');
  await settle(tester);
  return previews;
}

void main() {
  testWidgets('the web build ignores the link server', (tester) async {
    final store = MemoryLocalStore(serverUrl: _home);
    final previews = await _open(
      tester,
      store,
      _other,
      overrides: [homeServerProvider.overrideWithValue(_home)],
    );

    expect(previews, [_home]);
    await tester.tap(find.byKey(const Key('join-sign-in')));
    await settle(tester);
    expect(find.byKey(const Key('join-confirm-server')), findsNothing);
    expect(store.serverUrl, _home);
  });

  testWidgets('an installed app asks before switching servers', (tester) async {
    final store = MemoryLocalStore(serverUrl: _home);
    final previews = await _open(tester, store, _other);

    expect(previews, [_other]);
    await tester.tap(find.byKey(const Key('join-sign-in')));
    await settle(tester);
    expect(find.textContaining('This invite is for $_other'), findsOneWidget);
    expect(find.textContaining('instead of $_home'), findsOneWidget);
    await tester.tap(find.text('Cancel'));
    await settle(tester);
    expect(store.serverUrl, _home);
    expect(find.text('Join Flat'), findsOneWidget);

    await tester.tap(find.byKey(const Key('join-sign-in')));
    await settle(tester);
    await tester.tap(find.byKey(const Key('join-confirm-server')));
    await settle(tester);
    expect(store.serverUrl, _other);
    expect(find.byKey(const Key('login-email')), findsOneWidget);
  });

  testWidgets('plain http outside the local network is refused', (
    tester,
  ) async {
    final store = MemoryLocalStore(serverUrl: _home);
    final previews = await _open(tester, store, 'http://other.example.com');

    expect(previews, isEmpty);
    expect(find.text('This link is not safe to open'), findsOneWidget);
    expect(store.serverUrl, _home);
  });
}
