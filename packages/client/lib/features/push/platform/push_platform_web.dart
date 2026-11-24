import 'dart:async';
import 'dart:convert';
import 'dart:js_interop';
import 'dart:js_interop_unsafe';
import 'dart:typed_data';

import 'package:web/web.dart' as web;

import 'package:splitspends/features/push/domain/push_device.dart';
import 'package:splitspends/features/push/platform/push_platform.dart';

PushPlatform devicePushPlatform() => WebPushPlatform();

Future<bool> runPushBackground(List<String> args) async => false;

/// The browser's Push API with `push_sw.js` (in `apps/web/web/`) showing
/// the notifications. The worker gets its own scope (`push/`) so it never
/// controls pages or competes with Flutter's service worker.
class WebPushPlatform implements PushPlatform {
  static bool get _iOS {
    final ua = web.window.navigator.userAgent;
    return ua.contains('iPhone') || ua.contains('iPad');
  }

  bool get _supported =>
      web.window.isSecureContext &&
      (web.window.navigator as JSObject).has('serviceWorker') &&
      (web.window as JSObject).has('PushManager') &&
      (web.window as JSObject).has('Notification');

  @override
  String? get unsupportedReason {
    if (_supported) return null;
    if (_iOS) {
      return 'On iPhone and iPad, add Split Spends to your Home Screen from '
          "Safari's Share menu, open it from there and enable push again.";
    }
    if (!web.window.isSecureContext) {
      return 'Push notifications need the app to be opened over HTTPS.';
    }
    return 'This browser cannot receive push notifications.';
  }

  @override
  bool get usesDistributors => false;

  @override
  Future<List<Distributor>> distributors() async => const [];

  Uri get _base => Uri.parse(web.document.baseURI);
  String get _scope => _base.resolve('push/').toString();

  Future<web.ServiceWorkerRegistration?> _registration() async {
    if (!_supported) return null;
    return web.window.navigator.serviceWorker.getRegistration(_scope).toDart;
  }

  @override
  Future<PushKeys?> current() async {
    final registration = await _registration();
    if (registration == null) return null;
    final sub = await registration.pushManager.getSubscription().toDart;
    if (sub == null || web.Notification.permission != 'granted') return null;
    return _keys(sub);
  }

  @override
  Future<PushKeys> subscribe({
    required String vapidKey,
    Distributor? via,
  }) async {
    final reason = unsupportedReason;
    if (reason != null) throw PushError(reason);
    final permission =
        (await web.Notification.requestPermission().toDart).toDart;
    if (permission != 'granted') {
      throw const PushError(
        'Notifications are blocked for this site. Allow them in the '
        "browser's site settings and try again.",
      );
    }
    final registration = await web.window.navigator.serviceWorker
        .register(
          _base.resolve('push_sw.js').toString().toJS,
          web.RegistrationOptions(scope: _scope),
        )
        .toDart;
    await _activated(registration);
    final key = _decodeKey(vapidKey);
    final existing = await registration.pushManager.getSubscription().toDart;
    if (existing != null && !_sameKey(existing, key)) {
      // Subscribed with an older server key: browsers refuse a second
      // subscription with a different key until the first one is gone.
      await existing.unsubscribe().toDart;
    }
    try {
      final sub = await registration.pushManager
          .subscribe(
            web.PushSubscriptionOptionsInit(
              userVisibleOnly: true,
              applicationServerKey: key.toJS,
            ),
          )
          .toDart;
      return _keys(sub);
    } catch (error) {
      throw PushError(
        'The browser could not subscribe to its push service. '
        'Check your connection and try again. ($error)',
      );
    }
  }

  @override
  Future<void> unsubscribe() async {
    final registration = await _registration();
    final sub = await registration?.pushManager.getSubscription().toDart;
    await sub?.unsubscribe().toDart;
  }

  @override
  void watch(void Function(PushKeys keys) onChange) {
    // Browsers report subscription changes to the service worker only, and
    // the app re-registers its current subscription each time it starts.
  }

  /// Waits until the worker is active; subscribing needs an active worker.
  Future<void> _activated(web.ServiceWorkerRegistration registration) async {
    for (var i = 0; i < 100 && registration.active == null; i++) {
      await Future<void>.delayed(const Duration(milliseconds: 100));
    }
    if (registration.active == null) {
      throw const PushError(
        'The notification helper did not start. Reload the page and try '
        'again.',
      );
    }
  }

  PushKeys _keys(web.PushSubscription sub) {
    String key(String name) {
      final buffer = sub.getKey(name);
      if (buffer == null) {
        throw const PushError(
          'The browser returned an incomplete subscription.',
        );
      }
      return base64Url.encode(buffer.toDart.asUint8List()).replaceAll('=', '');
    }

    return PushKeys(
      endpoint: sub.endpoint,
      p256dh: key('p256dh'),
      auth: key('auth'),
      platform: 'web',
      label: browserLabel(web.window.navigator.userAgent),
    );
  }

  static Uint8List _decodeKey(String key) =>
      base64Url.decode(base64Url.normalize(key));

  static bool _sameKey(web.PushSubscription sub, Uint8List key) {
    final current = sub.options.applicationServerKey;
    if (current == null) return false;
    final bytes = current.toDart.asUint8List();
    if (bytes.length != key.length) return false;
    for (var i = 0; i < key.length; i++) {
      if (bytes[i] != key[i]) return false;
    }
    return true;
  }
}
