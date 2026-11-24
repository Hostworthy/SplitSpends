import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/models/user.dart';

/// How the app stores data.
enum AppMode {
  /// Not chosen yet: show the welcome screen.
  unset,

  /// No account; data stays on this device.
  local,

  /// Signed in to a SplitSpends server (sync, sharing, emails).
  connected;

  static AppMode parse(String? value) =>
      AppMode.values.where((m) => m.name == value).firstOrNull ?? AppMode.unset;
}

/// The current mode, server and signed-in (or on-device) user.
class Session {
  const Session({
    this.mode = AppMode.unset,
    this.serverUrl = '',
    this.token,
    this.me,
  });

  final AppMode mode;
  final String serverUrl;
  final String? token;
  final Me? me;

  /// True when the main app can be shown.
  bool get isReady =>
      mode == AppMode.local || (mode == AppMode.connected && token != null);

  bool get isLocal => mode == AppMode.local;
  bool get isConnected => mode == AppMode.connected && token != null;

  Session copyWith({AppMode? mode, String? serverUrl, Me? me}) => Session(
    mode: mode ?? this.mode,
    serverUrl: serverUrl ?? this.serverUrl,
    token: token,
    me: me ?? this.me,
  );
}

final localStoreProvider = Provider<LocalStore>((ref) => DeviceLocalStore());

/// Shared HTTP client; overridable in tests.
final httpClientProvider = Provider<http.Client>((ref) => http.Client());

/// The default server: the page origin on web (the backend serves the app),
/// nothing on mobile.
String defaultServerUrl() => kIsWeb ? Uri.base.origin : '';

final sessionProvider = AsyncNotifierProvider<SessionController, Session>(
  SessionController.new,
);

class SessionController extends AsyncNotifier<Session> {
  LocalStore get _store => ref.read(localStoreProvider);

  @override
  Future<Session> build() async {
    final serverUrl = (await _store.readServerUrl()) ?? defaultServerUrl();
    final token = await _store.readToken();
    var mode = AppMode.parse(await _store.readAppMode());
    if (mode == AppMode.unset && token != null) mode = AppMode.connected;

    if (mode == AppMode.local) {
      return Session(
        mode: mode,
        serverUrl: serverUrl,
        me: await _localProfile(),
      );
    }
    if (mode != AppMode.connected || token == null || serverUrl.isEmpty) {
      return Session(mode: mode, serverUrl: serverUrl);
    }
    final client = ApiClient(
      serverUrl: serverUrl,
      token: token,
      httpClient: ref.read(httpClientProvider),
    );
    try {
      final me = await client.getData('/me', (d) => Me.fromJson(d as JsonMap));
      return Session(mode: mode, serverUrl: serverUrl, token: token, me: me);
    } on ApiError catch (error) {
      if (error.isUnauthorized) {
        await _store.writeToken(null);
        return Session(mode: mode, serverUrl: serverUrl);
      }
      // Offline: keep the session, the profile loads later.
      return Session(mode: mode, serverUrl: serverUrl, token: token);
    }
  }

  Session get _current => state.value ?? Session(serverUrl: defaultServerUrl());

  /// "Use on this device": no account, data stays local.
  Future<void> useLocally({
    required String name,
    String currency = 'USD',
  }) async {
    final me = Me(
      id: 'local-${_randomId()}',
      email: '',
      name: name,
      defaultCurrency: currency,
    );
    await _store.writeLocalProfile(jsonEncode(_profileJson(me)));
    await _store.writeAppMode(AppMode.local.name);
    state = AsyncData(
      Session(mode: AppMode.local, serverUrl: _current.serverUrl, me: me),
    );
  }

  Future<void> setServerUrl(String url) async {
    final normalized = normalizeServerUrl(url);
    await _store.writeServerUrl(normalized);
    state = AsyncData(_current.copyWith(serverUrl: normalized));
  }

  /// Completes "Connect to a server" after the emailed code is verified.
  Future<void> signIn({required String token, required Me me}) async {
    await _store.writeToken(token);
    await _store.writeAppMode(AppMode.connected.name);
    state = AsyncData(
      Session(
        mode: AppMode.connected,
        serverUrl: _current.serverUrl,
        token: token,
        me: me,
      ),
    );
  }

  /// Saves profile changes (locally, or after the server accepted them).
  Future<void> updateMe(Me me) async {
    if (_current.isLocal) {
      await _store.writeLocalProfile(jsonEncode(_profileJson(me)));
    }
    state = AsyncData(_current.copyWith(me: me));
  }

  /// Signs out of the server and returns to the welcome screen. On-device
  /// data is kept.
  Future<void> signOut({bool revoke = true}) async {
    final session = _current;
    if (revoke && session.token != null) {
      final client = ApiClient(
        serverUrl: session.serverUrl,
        token: session.token,
        httpClient: ref.read(httpClientProvider),
      );
      try {
        await client.post('/auth/logout');
      } catch (_) {
        // Best effort.
      }
    }
    await _store.writeToken(null);
    await _store.writeAppMode(null);
    state = AsyncData(Session(serverUrl: session.serverUrl));
  }

  /// Back to the welcome screen without touching stored data.
  Future<void> chooseAgain() async {
    await _store.writeAppMode(null);
    state = AsyncData(Session(serverUrl: _current.serverUrl));
  }

  Future<Me?> _localProfile() async {
    final raw = await _store.readLocalProfile();
    if (raw == null) return null;
    return Me.fromJson(jsonDecode(raw) as JsonMap);
  }

  static JsonMap _profileJson(Me me) => {
    ...me.toJson(),
    'timezone': me.timezone,
    'notifications': me.notifications.toJson(),
  };

  static String _randomId() {
    final random = Random.secure();
    return List.generate(
      16,
      (_) => random.nextInt(256).toRadixString(16).padLeft(2, '0'),
    ).join();
  }
}

/// API client bound to the current server and token. Rebuilds only when the
/// server or token changes, not when the profile changes.
final apiClientProvider = Provider<ApiClient>((ref) {
  final key = ref.watch(
    sessionProvider.select((s) => (s.value?.serverUrl, s.value?.token)),
  );
  return ApiClient(
    serverUrl: key.$1 ?? defaultServerUrl(),
    token: key.$2,
    httpClient: ref.read(httpClientProvider),
    onUnauthorized: () {
      if (key.$2 != null) {
        ref.read(sessionProvider.notifier).signOut(revoke: false);
      }
    },
  );
});

/// The current mode (local, connected, or not chosen yet).
final appModeProvider = Provider<AppMode>(
  (ref) =>
      ref.watch(sessionProvider.select((s) => s.value?.mode)) ?? AppMode.unset,
);

/// The signed-in (or on-device) user, null while unknown.
final meProvider = Provider<Me?>((ref) => ref.watch(sessionProvider).value?.me);
