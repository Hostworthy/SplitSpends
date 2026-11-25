import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:sembast/sembast.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/features/push/domain/push_device.dart';
import 'package:splitspends/features/push/domain/push_repository.dart';
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

/// The subscription this device last sent to the server.
const _sentKey = 'pushSent';

/// Registers [keys] with the server and remembers them as sent.
Future<void> _register(
  PushRepository repo,
  Future<Database> database,
  PushKeys keys,
) async {
  await repo.register(keys);
  await Stores.meta.record(_sentKey).put(await database, keys.toJson());
}

/// Whether [keys] are the ones this device last sent to the server.
Future<bool> _sent(Future<Database> database, PushKeys keys) async {
  final raw = await Stores.meta.record(_sentKey).get(await database);
  if (raw is! Map) return false;
  final sent = PushKeys.fromJson(raw.cast<String, Object?>());
  return sent.endpoint == keys.endpoint &&
      sent.p256dh == keys.p256dh &&
      sent.auth == keys.auth;
}

/// Keeps the server's copy of this device's subscription current: when the
/// UnifiedPush distributor hands out a new endpoint while the app runs, and
/// on start when the push service replaced the subscription while the app
/// was closed (browsers tell only the service worker). An unchanged
/// subscription is not sent again, so a device removed from another device
/// is not added back.
final pushSyncProvider = FutureProvider<void>((ref) async {
  if (!_connected(ref)) return;
  final repo = ref.watch(pushRepositoryProvider);
  final database = ref.watch(databaseProvider.future);
  final platform = ref.watch(pushPlatformProvider);
  Future<void> send(PushKeys keys) async {
    try {
      await _register(repo, database, keys);
      if (ref.mounted) ref.invalidate(pushDevicesProvider);
    } catch (_) {
      // Retried on the next start or when the endpoint changes again.
    }
  }

  platform.watch(send);
  try {
    final keys = await platform.current();
    if (keys != null && !await _sent(database, keys)) await send(keys);
  } catch (_) {
    // Tried again on the next start.
  }
});

/// Push belongs to a signed-in account, so this device stops receiving it
/// whenever no account is signed in: after signing out, and when the server
/// ends the session (expired, revoked or disabled), which leaves no chance
/// to tell the server first.
final pushSignOutProvider = Provider<void>((ref) {
  ref.listen(sessionProvider, (_, next) async {
    final session = next.value;
    if (session == null || session.isConnected) return;
    final platform = ref.read(pushPlatformProvider);
    try {
      if (await platform.current() != null) await platform.unsubscribe();
    } catch (_) {
      // Tried again on the next start.
    }
  }, fireImmediately: true);
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
      await _register(
        _ref.read(pushRepositoryProvider),
        _ref.read(databaseProvider.future),
        keys,
      );
    } finally {
      _refresh();
    }
  }

  /// Unsubscribes this device and removes it from the server. The device
  /// is unsubscribed even when the server cannot be told: the server drops
  /// the subscription once the push service reports it gone.
  Future<void> disable() async {
    try {
      final current = await _platform.current();
      if (current == null) return;
      try {
        final repo = _ref.read(pushRepositoryProvider);
        final devices = await repo.list();
        for (final d in devices.where((d) => d.endpoint == current.endpoint)) {
          await repo.remove(d.id);
        }
      } finally {
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
