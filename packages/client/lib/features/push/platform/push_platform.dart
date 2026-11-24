import 'package:splitspends/features/push/domain/push_device.dart';

import 'package:splitspends/features/push/platform/push_platform_io.dart'
    if (dart.library.js_interop) 'package:splitspends/features/push/platform/push_platform_web.dart'
    as impl;

/// How this device receives push: the browser's Push API on the web,
/// UnifiedPush on Android. Both produce a standard Web Push subscription.
/// The web build never imports the Android plugin (conditional import).
abstract interface class PushPlatform {
  /// Why push cannot work here, or null when it can.
  String? get unsupportedReason;

  /// True on Android, where the user picks a UnifiedPush distributor.
  bool get usesDistributors;

  /// Installed UnifiedPush distributors (Android only).
  Future<List<Distributor>> distributors();

  /// This device's subscription, when push is on here.
  Future<PushKeys?> current();

  /// Asks for permission and subscribes with the server's VAPID key.
  /// Throws [PushError] with a readable message.
  Future<PushKeys> subscribe({required String vapidKey, Distributor? via});

  /// Stops push on this device.
  Future<void> unsubscribe();

  /// Calls [onChange] when the push service hands out a new endpoint on its
  /// own (UnifiedPush distributors may rotate it), so the server can be told.
  void watch(void Function(PushKeys keys) onChange);
}

/// The implementation for this platform.
PushPlatform devicePushPlatform() => impl.devicePushPlatform();

/// Handles push events when Android starts the app in the background
/// (`--unifiedpush-bg`). Returns true when it did, so main skips the UI.
Future<bool> runPushBackground(List<String> args) =>
    impl.runPushBackground(args);
