import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;
import 'package:sembast/sembast.dart';
import 'package:uuid/uuid.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/response_cache.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/core/sync/reachability.dart';

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
    this.userId,
  });

  final AppMode mode;
  final String serverUrl;
  final String? token;
  final Me? me;

  /// The signed-in account's id, known before [me] loads on an offline
  /// start.
  final String? userId;

  /// Who writes queued now belong to; null when not signed in.
  Owner? get owner => token == null
      ? null
      : (server: normalizeServerUrl(serverUrl), user: me?.id ?? userId);

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
    userId: userId,
  );
}

final localStoreProvider = Provider<LocalStore>((ref) => DeviceLocalStore());

/// Shared HTTP client; overridable in tests.
final httpClientProvider = Provider<http.Client>((ref) => http.Client());

/// The default server: the page origin on web (the backend serves the app),
/// nothing on mobile.
String defaultServerUrl() {
  // `--dart-define=SPLITSPENDS_URL=http://localhost:8080` points a dev build
  // at a separately running API (see `make web`).
  const configured = String.fromEnvironment('SPLITSPENDS_URL');
  if (configured.isNotEmpty) return configured;
  return kIsWeb ? Uri.base.origin : '';
}

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
    final client = _client(serverUrl, token);
    final db = await ref.read(databaseProvider.future);
    final account = await Stores.meta.record(_accountKey).get(db) as String?;
    try {
      final me = await client.getData('/me', (d) => Me.fromJson(d as JsonMap));
      final session = Session(
        mode: mode,
        serverUrl: serverUrl,
        token: token,
        me: me,
        userId: me.id,
      );
      if (account == null) {
        // The first start that knows the account: writes queued before it
        // was recorded were made with this same token.
        await _outbox(session).claim();
      }
      await _remember(me);
      return session;
    } on ApiError catch (error) {
      if (error.isUnauthorized) {
        // Queued writes stay with their account until it signs in again.
        await _store.writeToken(null);
        return Session(mode: mode, serverUrl: serverUrl);
      }
      // Offline: keep the session with the profile this device last saw,
      // so people can add expenses right away.
      final raw = await Stores.meta.record(_profileKey).get(db);
      final known = raw == null ? null : Me.fromJson(raw as JsonMap);
      ref.listen(reachabilityProvider, (was, now) {
        if (now && was == false) _reload();
      });
      return Session(
        mode: mode,
        serverUrl: serverUrl,
        token: token,
        me: known?.id == account ? known : null,
        userId: account,
      );
    }
  }

  /// The signed-in account's id (see [Session.userId]).
  static const _accountKey = 'account';

  /// The signed-in account's last known profile, for offline starts.
  static const _profileKey = 'accountProfile';

  /// Loads the profile an offline start did not have, once the server is
  /// reachable again.
  Future<void> _reload() async {
    final session = state.value;
    if (session == null || !session.isConnected || session.me != null) return;
    final Me me;
    try {
      me = await _client(
        session.serverUrl,
        session.token!,
      ).getData('/me', (d) => Me.fromJson(d as JsonMap));
    } on ApiError {
      return; // Tried again when the server is next reachable.
    }
    final now = state.value;
    if (now == null || now.token != session.token || now.me != null) return;
    final known = now.copyWith(me: me);
    if (now.userId == null) await _outbox(known).claim();
    await _remember(me);
    state = AsyncData(known);
  }

  /// A client for loading the profile; it reads the cached copy offline.
  ApiClient _client(String serverUrl, String token) => ApiClient(
    serverUrl: serverUrl,
    token: token,
    httpClient: ref.read(httpClientProvider),
    cache: ref.read(responseCacheProvider),
  );

  /// Records [me] as the signed-in account and keeps its profile.
  Future<void> _remember(Me me) async {
    final db = await ref.read(databaseProvider.future);
    await Stores.meta.record(_accountKey).put(db, me.id);
    await Stores.meta.record(_profileKey).put(db, _profileJson(me));
  }

  Outbox _outbox(Session session) =>
      Outbox(() => ref.read(databaseProvider.future), owner: session.owner);

  Session get _current => state.value ?? Session(serverUrl: defaultServerUrl());

  /// "Use on this device": no account, data stays local. Reuses an existing
  /// on-device profile so its data stays attached.
  Future<void> useLocally({
    required String name,
    String currency = 'USD',
  }) async {
    final existing = await _localProfile();
    final me =
        existing?.copyWith(name: name, defaultCurrency: currency) ??
        Me(id: const Uuid().v4(), name: name, defaultCurrency: currency);
    await _saveLocalProfile(me);
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
  /// Writes this account queued before its session ended are replayed;
  /// those of other accounts stay untouched (see [Outbox]).
  Future<void> signIn({required String token, required Me me}) async {
    await ref.read(responseCacheProvider).clear();
    await _remember(me);
    await _store.writeToken(token);
    await _store.writeAppMode(AppMode.connected.name);
    state = AsyncData(
      Session(
        mode: AppMode.connected,
        serverUrl: _current.serverUrl,
        token: token,
        me: me,
        userId: me.id,
      ),
    );
  }

  /// Saves profile changes (locally, or after the server accepted them).
  Future<void> updateMe(Me me) async {
    if (_current.isLocal) await _saveLocalProfile(me);
    if (_current.isConnected) await _remember(me);
    state = AsyncData(_current.copyWith(me: me));
  }

  /// Signs out of the server at the user's request and returns to the
  /// welcome screen, discarding the account's unsynced writes (the account
  /// screen warns about them first). On-device data is kept.
  Future<void> signOut() async {
    final session = _current;
    if (session.token != null) {
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
    try {
      await ref.read(responseCacheProvider).clear();
      await _outbox(session).clear();
      final db = await ref.read(databaseProvider.future);
      await Stores.meta.record(_profileKey).delete(db);
    } catch (_) {
      // Leftover writes stay bound to this account and are never sent for
      // anyone else; the cache is cleared again on the next sign-in.
    }
    state = AsyncData(Session(serverUrl: session.serverUrl));
  }

  /// The server no longer accepts [token] (it expired or was revoked):
  /// back to the welcome screen. Unlike [signOut], the account's unsynced
  /// writes are kept, so signing in to it again sends them.
  Future<void> expire(String token) async {
    final session = _current;
    if (session.token != token) return;
    state = AsyncData(Session(serverUrl: session.serverUrl));
    await _store.writeToken(null);
    await _store.writeAppMode(null);
    try {
      await ref.read(responseCacheProvider).clear();
    } catch (_) {
      // Cleared again on the next sign-in.
    }
  }

  /// Back to the welcome screen without touching stored data.
  Future<void> chooseAgain() async {
    await _store.writeAppMode(null);
    state = AsyncData(Session(serverUrl: _current.serverUrl));
  }

  Future<Me?> _localProfile() async {
    final db = await ref.read(databaseProvider.future);
    final raw = await Stores.meta.record('profile').get(db);
    return raw == null ? null : Me.fromJson(raw as JsonMap);
  }

  Future<void> _saveLocalProfile(Me me) async {
    final db = await ref.read(databaseProvider.future);
    await Stores.meta.record('profile').put(db, _profileJson(me));
  }

  static JsonMap _profileJson(Me me) => {
    ...me.toJson(),
    'timezone': me.timezone,
    'notifications': me.notifications.toJson(),
  };
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
    cache: key.$2 == null ? null : ref.read(responseCacheProvider),
    onReachability: (ok) => ref.read(reachabilityProvider.notifier).report(ok),
    onUnauthorized: () {
      final token = key.$2;
      if (token != null) ref.read(sessionProvider.notifier).expire(token);
    },
  );
});

/// Offline copies of server responses (connected mode).
final responseCacheProvider = Provider<ResponseCache>(
  (ref) => SembastResponseCache(() => ref.read(databaseProvider.future)),
);

/// The current mode (local, connected, or not chosen yet).
final appModeProvider = Provider<AppMode>(
  (ref) =>
      ref.watch(sessionProvider.select((s) => s.value?.mode)) ?? AppMode.unset,
);

/// The signed-in (or on-device) user, null while unknown.
final meProvider = Provider<Me?>((ref) => ref.watch(sessionProvider).value?.me);
