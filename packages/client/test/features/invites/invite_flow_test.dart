import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';

import '../../helpers.dart';

const _token = 'k3V9xQ2mN8pL4rT7wY1zA6bC0dE5fG8hJ2kM4nP6qR8';
const _server = 'http://splitspends.test';
const _me = Me(id: 'cy', email: 'cy@example.com', name: '');

class _Auth implements AuthRepository {
  final requested = <String>[];

  @override
  Future<int> requestCode(
    String serverUrl,
    String email, {
    String? inviteToken,
  }) async {
    requested.add('$serverUrl|$email|$inviteToken');
    return 600;
  }

  @override
  Future<LoginResult> verifyCode(String s, String e, String code) async =>
      const LoginResult(token: 'tok', me: _me, isNew: true);

  @override
  Future<Me> setName(String serverUrl, String token, String name) async =>
      Me(id: _me.id, email: _me.email, name: name);
}

const _group = {
  'id': 'g1',
  'name': 'Flat',
  'kind': 'home',
  'defaultCurrency': 'EUR',
  'simplifyDebts': true,
  'emoji': '🏠',
  'color': '#00897B',
  'archived': false,
  'members': [
    {'id': 'ada', 'email': 'ada@example.com', 'name': 'Ada'},
    {'id': 'cy', 'email': 'cy@example.com', 'name': 'Cy'},
  ],
  'balances': <Object>[],
};

void main() {
  testWidgets('invite link → sign in with the token → joined', (tester) async {
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    var accepted = 0;
    final api = MockClient((request) async {
      final path = request.url.path;
      Object body;
      if (path.endsWith('/invites/$_token') && request.method == 'GET') {
        body = {
          'data': {
            'groupName': 'Flat',
            'emoji': '🏠',
            'color': '#00897B',
            'memberCount': 1,
            'invitedBy': 'Ada',
          },
        };
      } else if (path.endsWith('/invites/$_token/accept')) {
        expect(request.headers['Authorization'], 'Bearer tok');
        accepted++;
        body = {'data': _group};
      } else if (path.endsWith('/groups/g1')) {
        body = {'data': _group};
      } else if (path.endsWith('/groups/g1/balances')) {
        body = {
          'data': {'members': <Object>[], 'debts': <Object>[]},
        };
      } else if (path.endsWith('/me/balances')) {
        body = {
          'data': {'balances': <Object>[], 'owed': <Object>[], 'owing': []},
        };
      } else if (path.endsWith('/config')) {
        body = {
          'data': {
            'appName': 'SplitSpends',
            'aiEnabled': false,
            'signupMode': 'invite',
            'currencies': [
              {'code': 'EUR', 'name': 'Euro', 'symbol': '€', 'exponent': 2},
            ],
            'categories': <Object>[],
          },
        };
      } else {
        body = {'items': <Object>[], 'total': 0};
      }
      return http.Response(
        jsonEncode(body),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final auth = _Auth();
    final store = MemoryLocalStore();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(store),
          memoryDatabase(),
          authRepositoryProvider.overrideWithValue(auth),
          httpClientProvider.overrideWithValue(api),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await tester.pumpAndSettle();
    final container = ProviderScope.containerOf(
      tester.element(find.byType(SplitSpendsApp)),
    );
    final router = container.read(routerProvider);
    router.go('/join/$_token?server=${Uri.encodeQueryComponent(_server)}');
    await tester.pumpAndSettle();

    // Signed out: the preview shows and offers to sign in.
    expect(find.text('Join Flat'), findsOneWidget);
    expect(find.textContaining('Ada invited you'), findsOneWidget);
    await tester.tap(find.byKey(const Key('join-sign-in')));
    await tester.pumpAndSettle();

    // The server comes from the link; the token goes with the code request.
    expect(find.textContaining('to join the group'), findsOneWidget);
    await tester.enterText(
      find.byKey(const Key('login-email')),
      'cy@example.com',
    );
    await tester.tap(find.byKey(const Key('login-continue')));
    await tester.pumpAndSettle();
    expect(auth.requested, ['$_server|cy@example.com|$_token']);

    await tester.enterText(find.byKey(const Key('otp-field')), '123456');
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));
    await tester.enterText(find.byKey(const Key('name-field')), 'Cy');
    await tester.tap(find.text('Get started'));
    for (var i = 0; i < 10; i++) {
      await tester.pump(const Duration(milliseconds: 200));
    }

    // Back on the link after signing in, the app joins by itself.
    expect(accepted, 1);
    expect(router.routerDelegate.currentConfiguration.uri.path, '/groups/g1');
  });
}
