import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:unifiedpush_android/unifiedpush_android.dart';
import 'package:unifiedpush_platform_interface/data/failed_reason.dart';
import 'package:unifiedpush_platform_interface/data/push_endpoint.dart';
import 'package:unifiedpush_platform_interface/unifiedpush_platform_interface.dart'
    as up;

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/features/push/data/remote_push_repository.dart';
import 'package:splitspends/features/push/domain/push_device.dart';
import 'package:splitspends/features/push/platform/push_platform.dart';

PushPlatform devicePushPlatform() {
  if (Platform.isAndroid) return UnifiedPushDevice();
  return UnavailablePushPlatform(
    Platform.isIOS
        ? 'Push notifications on iPhone are not available yet.'
        : 'Push notifications are not available on this device yet.',
  );
}

/// Android starts the app with `--unifiedpush-bg` when a push event arrives
/// while it is closed. Messages are shown natively (see PushService.kt);
/// here only a new or removed endpoint needs handling, so the server keeps
/// an up-to-date address for this phone.
Future<bool> runPushBackground(List<String> args) async {
  if (!Platform.isAndroid || !args.contains('--unifiedpush-bg')) return false;
  WidgetsFlutterBinding.ensureInitialized();
  final push = UnifiedPushDevice();
  push.watch((keys) async {
    final store = DeviceLocalStore();
    final server = await store.readServerUrl();
    final token = await store.readToken();
    if (server == null || server.isEmpty || token == null) return;
    try {
      await RemotePushRepository(ApiClient(serverUrl: server, token: token))
          .register(keys);
    } catch (_) {
      // The app re-registers on its next start.
    }
  });
  await push._ready();
  return true;
}

/// Push is not offered on this platform.
class UnavailablePushPlatform implements PushPlatform {
  const UnavailablePushPlatform(this.unsupportedReason);

  @override
  final String unsupportedReason;

  @override
  bool get usesDistributors => false;

  @override
  Future<List<Distributor>> distributors() async => const [];

  @override
  Future<PushKeys?> current() async => null;

  @override
  Future<PushKeys> subscribe({required String vapidKey, Distributor? via}) =>
      throw PushError(unsupportedReason);

  @override
  Future<void> unsubscribe() async {}

  @override
  void watch(void Function(PushKeys keys) onChange) {}
}

/// UnifiedPush in Web Push mode: the connector library generates the
/// encryption keys and decrypts messages, the distributor (ntfy, NextPush,
/// an FCM relay, ...) delivers them. The app never talks to Google.
class UnifiedPushDevice implements PushPlatform {
  UnifiedPushDevice();

  static const _instance = 'splitspends';
  static const _savedKey = 'push.unifiedpush.subscription';
  static const _channel = MethodChannel('dev.splitspends.app/push');

  static Completer<PushKeys>? _pending;
  static void Function(PushKeys keys)? _onChange;
  static Future<void>? _initialized;

  /// The plugin; registered here too in case the generated plugin
  /// registrant did not (it is a transitive dependency of the app).
  static up.UnifiedPushPlatform get _plugin {
    if (up.UnifiedPushPlatform.instance is! UnifiedPushAndroid) {
      UnifiedPushAndroid.registerWith();
    }
    return up.UnifiedPushPlatform.instance;
  }

  /// Callbacks must be in place before registering, or the first endpoint
  /// is lost.
  Future<void> _ready() => _initialized ??= _plugin.initializeCallback(
    onNewEndpoint: _endpoint,
    onRegistrationFailed: _failed,
    onUnregistered: (_) => _forget(),
  );

  static Future<void> _endpoint(PushEndpoint endpoint, String instance) async {
    final keySet = endpoint.pubKeySet;
    if (keySet == null) {
      _fail(
        'The distributor did not provide encryption keys. Update it, or '
        'pick another distributor.',
      );
      return;
    }
    final keys = PushKeys(
      endpoint: endpoint.url,
      p256dh: keySet.pubKey,
      auth: keySet.auth,
      platform: 'unifiedpush',
      label: await _label(),
    );
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_savedKey, jsonEncode(keys.toJson()));
    final pending = _pending;
    _pending = null;
    if (pending != null && !pending.isCompleted) {
      pending.complete(keys);
    } else {
      _onChange?.call(keys);
    }
  }

  static void _failed(FailedReason reason, String instance) =>
      _fail(switch (reason) {
        FailedReason.network =>
          'The distributor is offline. Check your connection and try again.',
        FailedReason.actionRequired =>
          'The distributor needs your attention. Open it, then try again.',
        FailedReason.vapidRequired =>
          'The distributor needs a server key. Update Split Spends and try '
              'again.',
        FailedReason.internalError =>
          'The distributor could not register Split Spends. Try again.',
      });

  static void _fail(String message) {
    final pending = _pending;
    _pending = null;
    if (pending != null && !pending.isCompleted) {
      pending.completeError(PushError(message));
    }
  }

  static Future<void> _forget() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_savedKey);
  }

  static Future<String> _label() async {
    final id = await _plugin.getDistributor();
    return id == null ? 'Android' : 'Android via ${distributorName(id)}';
  }

  @override
  String? get unsupportedReason => null;

  @override
  bool get usesDistributors => true;

  @override
  Future<List<Distributor>> distributors() async {
    final ids = await _plugin.getDistributors(const []);
    return [
      for (final id in ids)
        if (id != 'dev.splitspends.app') Distributor(id),
    ];
  }

  @override
  Future<PushKeys?> current() async {
    if (await _plugin.getDistributor() == null) {
      return null;
    }
    final raw = (await SharedPreferences.getInstance()).getString(_savedKey);
    if (raw == null) return null;
    return PushKeys.fromJson(jsonDecode(raw) as Map<String, Object?>);
  }

  @override
  Future<PushKeys> subscribe({
    required String vapidKey,
    Distributor? via,
  }) async {
    if (via == null) {
      throw const PushError(
        'Install a UnifiedPush distributor app to get push notifications.',
        needsDistributor: true,
      );
    }
    final allowed = await _channel.invokeMethod<bool>('requestPermission');
    if (allowed != true) {
      throw const PushError(
        'Notifications are turned off for Split Spends. Allow them in '
        'Android settings and try again.',
      );
    }
    await _ready();
    final api = _plugin;
    await api.saveDistributor(via.id);
    final pending = _pending = Completer<PushKeys>();
    await api.register(_instance, const [], null, vapidKey);
    return pending.future.timeout(
      const Duration(seconds: 30),
      onTimeout: () {
        _pending = null;
        throw PushError(
          '${via.name} did not answer. Open it to check that it is '
          'connected, then try again.',
        );
      },
    );
  }

  @override
  Future<void> unsubscribe() async {
    await _ready();
    await _plugin.unregister(_instance);
    await _forget();
  }

  @override
  void watch(void Function(PushKeys keys) onChange) {
    _onChange = onChange;
    unawaited(_ready());
  }
}
