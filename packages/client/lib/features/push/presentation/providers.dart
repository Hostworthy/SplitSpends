import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/features/push/domain/push_device.dart';
import 'package:splitspends/features/push/platform/push_platform.dart';
import 'package:splitspends/features/push/presentation/push_view.dart';

/// How this device receives push; overridable in tests.
final pushPlatformProvider = Provider<PushPlatform>(
  (ref) => devicePushPlatform(),
);

bool _connected(Ref ref) =>
    ref.watch(sessionProvider).value?.isConnected ?? false;

/// Devices registered on the server.
final pushDevicesProvider = FutureProvider<List<PushDevice>>((ref) async {
  if (!_connected(ref) || !ref.watch(configValueProvider).pushEnabled) {
    return const [];
  }
  return ref.watch(pushRepositoryProvider).list();
});

/// This device's own subscription, if any.
final pushCurrentProvider = FutureProvider<PushKeys?>(
  (ref) => ref.watch(pushPlatformProvider).current(),
);

/// Everything the Account screen shows about push.
final pushViewProvider = FutureProvider<PushView>((ref) async {
  final platform = ref.watch(pushPlatformProvider);
  final enabled = ref.watch(configValueProvider).pushEnabled;
  return PushView.resolve(
    connected: _connected(ref),
    serverEnabled: enabled,
    unsupportedReason: platform.unsupportedReason,
    current: await ref.watch(pushCurrentProvider.future),
    devices: await ref.watch(pushDevicesProvider.future),
  );
});

/// Keeps the server's copy of this phone's endpoint current when the
/// UnifiedPush distributor hands out a new one while the app runs.
final pushSyncProvider = Provider<void>((ref) {
  if (!_connected(ref)) return;
  final repo = ref.watch(pushRepositoryProvider);
  ref.watch(pushPlatformProvider).watch((keys) async {
    try {
      await repo.register(keys);
      ref.invalidate(pushDevicesProvider);
    } catch (_) {
      // Retried when the distributor reports the endpoint again.
    }
  });
});

final pushActionsProvider = Provider<PushActions>(PushActions.new);

/// Turning push on and off and managing devices.
class PushActions {
  PushActions(this._ref);

  final Ref _ref;

  PushPlatform get _platform => _ref.read(pushPlatformProvider);

  void _refresh() {
    _ref.invalidate(pushCurrentProvider);
    _ref.invalidate(pushDevicesProvider);
  }

  /// Subscribes this device and registers it with the server.
  Future<void> enable({Distributor? via}) async {
    final key = _ref.read(configValueProvider).vapidPublicKey;
    final keys = await _platform.subscribe(vapidKey: key, via: via);
    try {
      await _ref.read(pushRepositoryProvider).register(keys);
    } finally {
      _refresh();
    }
  }

  /// Unsubscribes this device and removes it from the server.
  Future<void> disable() async {
    try {
      final current = await _platform.current();
      if (current != null) {
        final devices = await _ref.read(pushRepositoryProvider).list();
        for (final d in devices.where((d) => d.endpoint == current.endpoint)) {
          await _ref.read(pushRepositoryProvider).remove(d.id);
        }
        await _platform.unsubscribe();
      }
    } finally {
      _refresh();
    }
  }

  /// Removes a device; for this device push is turned off here too.
  Future<void> remove(PushDevice device, {required bool here}) async {
    try {
      await _ref.read(pushRepositoryProvider).remove(device.id);
      if (here) await _platform.unsubscribe();
    } finally {
      _refresh();
    }
  }

  Future<PushTestResult> test() async {
    try {
      return await _ref.read(pushRepositoryProvider).test();
    } finally {
      _ref.invalidate(pushDevicesProvider);
    }
  }

  /// Before signing out: this device should stop getting the account's
  /// notifications. Best effort.
  Future<void> forget() async {
    try {
      await disable();
    } catch (_) {
      // Signing out goes ahead regardless.
    }
  }
}
