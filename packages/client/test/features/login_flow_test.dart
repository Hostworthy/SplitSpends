import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';

import '../helpers.dart';

const _me = Me(
  id: 'u1',
  email: 'ada@example.com',
  name: '',
  defaultCurrency: 'EUR',
);

class FakeAuthRepository implements AuthRepository {
  final requested = <String>[];
  String? named;

  @override
  Future<int> requestCode(
    String serverUrl,
    String email, {
    String? inviteToken,
  }) async {
    requested.add(
      inviteToken == null
          ? '$serverUrl|$email'
          : '$serverUrl|$email|$inviteToken',
    );
    return 600;
  }

  @override
  Future<LoginResult> verifyCode(
    String serverUrl,
    String email,
    String code,
  ) async {
    if (code != '123456') {
      throw const ApiError(
        status: 401,
        message: 'That code is not valid.',
        code: 'UNAUTHORIZED',
      );
    }
    return const LoginResult(token: 'tok', me: _me, isNew: true);
  }

  @override
  Future<Me> setName(String serverUrl, String token, String name) async {
    named = name;
    return Me(id: _me.id, email: _me.email, name: name, defaultCurrency: 'EUR');
  }
}

/// Minimal API for the dashboard after login.
http.Client _api() => MockClient((request) async {
  final path = request.url.path;
  Object body;
  if (path.endsWith('/me/balances')) {
    body = {
      'data': {
        'balances': [
          {'currency': 'EUR', 'amount': 1250},
        ],
        'owed': [
          {'currency': 'EUR', 'amount': 1250},
        ],
        'owing': <Object>[],
      },
    };
  } else if (path.endsWith('/config')) {
    body = {
      'data': {
        'appName': 'SplitSpends',
        'aiEnabled': false,
        'signupMode': 'open',
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

void main() {
  testWidgets('welcome → connect → email → code → name → dashboard', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    final auth = FakeAuthRepository();
    final store = MemoryLocalStore();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(store),
          memoryDatabase(),
          authRepositoryProvider.overrideWithValue(auth),
          httpClientProvider.overrideWithValue(_api()),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await tester.pumpAndSettle();

    // Welcome: two equal choices; pick "Connect to a server".
    expect(find.text('Use on this device'), findsOneWidget);
    expect(find.text('Connect to a server'), findsOneWidget);
    await tester.ensureVisible(find.text('Connect'));
    await tester.tap(find.text('Connect'));
    await tester.pumpAndSettle();

    // Step 1: server + email (mobile has no default server).
    expect(find.byKey(const Key('login-email')), findsOneWidget);
    await tester.enterText(
      find.byKey(const Key('login-server')),
      'http://splitspends.test',
    );
    await tester.enterText(
      find.byKey(const Key('login-email')),
      'Ada@Example.com',
    );
    await tester.tap(find.byKey(const Key('login-continue')));
    await tester.pumpAndSettle();
    expect(auth.requested, ['http://splitspends.test|ada@example.com']);
    expect(store.serverUrl, 'http://splitspends.test');

    // Step 2: a wrong code shows an error, the right one moves on.
    expect(find.text('Check your inbox'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('otp-field')), '000000');
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('That code is not valid.'), findsOneWidget);

    await tester.enterText(find.byKey(const Key('otp-field')), '123456');
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    // Step 3: new account picks a name.
    expect(find.text('Welcome aboard'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('name-field')), 'Ada');
    await tester.tap(find.text('Get started'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pump(const Duration(milliseconds: 500));

    expect(auth.named, 'Ada');
    expect(store.token, 'tok');
    expect(find.textContaining('Ada'), findsWidgets);
    expect(find.text('You are owed'), findsOneWidget);
    expect(find.text('€12.50'), findsWidgets);
    expect(store.appMode, 'connected');
  });

  testWidgets('welcome → use on this device needs no account', (tester) async {
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    final auth = FakeAuthRepository();
    final store = MemoryLocalStore();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(store),
          memoryDatabase(),
          authRepositoryProvider.overrideWithValue(auth),
          httpClientProvider.overrideWithValue(_api()),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Start'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('local-name')), 'Grace');
    await tester.tap(find.byKey(const Key('local-start')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));

    expect(auth.requested, isEmpty);
    expect(store.appMode, 'local');
    expect(store.token, isNull);
    final container = ProviderScope.containerOf(
      tester.element(find.byType(SplitSpendsApp)),
    );
    final db = await pumpFuture(
      tester,
      container.read(databaseProvider.future),
    );
    final profile = await pumpFuture(
      tester,
      Stores.meta.record('profile').get(db),
    );
    expect((profile! as Map)['name'], 'Grace');
    expect(find.text('Grace'), findsWidgets);
  });
}
