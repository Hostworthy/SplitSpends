import 'package:splitspends/features/push/domain/push_device.dart';

/// Where push stands on this device.
enum PushState {
  /// On-device mode: push needs a server.
  needsServer,

  /// The server does not send push.
  serverOff,

  /// This platform or browser cannot receive push.
  unsupported,

  /// Push could work here but is off.
  off,

  /// This device is registered.
  on,
}

/// What the Account screen shows about push: computed from the session,
/// the server config, the platform and the registered devices, so it can be
/// tested without a device.
class PushView {
  const PushView._(this.state, this.message, this.devices, this.thisDevice);

  factory PushView.resolve({
    required bool connected,
    required bool serverEnabled,
    String? unsupportedReason,
    PushKeys? current,
    List<PushDevice> devices = const [],
  }) {
    if (!connected) {
      return const PushView._(
        PushState.needsServer,
        'Push notifications need a server. Connect this app to a '
        'Split Spends server to get them on your phone or in your browser.',
        [],
        null,
      );
    }
    if (!serverEnabled) {
      return PushView._(
        PushState.serverOff,
        'This server does not send push notifications.',
        devices,
        null,
      );
    }
    final mine = current == null
        ? null
        : devices.where((d) => d.endpoint == current.endpoint).firstOrNull;
    if (unsupportedReason != null) {
      return PushView._(
        PushState.unsupported,
        unsupportedReason,
        devices,
        null,
      );
    }
    if (mine != null) {
      return PushView._(
        PushState.on,
        'This device gets push notifications.',
        devices,
        mine.id,
      );
    }
    return PushView._(
      PushState.off,
      'Get notified on this device, even when the app is closed.',
      devices,
      null,
    );
  }

  final PushState state;
  final String message;

  /// Devices registered on the server, this one included.
  final List<PushDevice> devices;

  /// The id of this device in [devices], when push is on here.
  final String? thisDevice;

  bool get canEnable => state == PushState.off;
  bool get canDisable => state == PushState.on;

  /// A test needs at least one registered device.
  bool get canTest =>
      devices.isNotEmpty &&
      state != PushState.needsServer &&
      state != PushState.serverOff;

  /// Per-kind push switches only matter where the server can push.
  bool get showSwitches =>
      state != PushState.needsServer && state != PushState.serverOff;
}

/// Shown on Android when no UnifiedPush distributor is installed.
const noDistributorMessage =
    'Android delivers push notifications through a separate app called a '
    'UnifiedPush distributor, for example ntfy. Install one, open it once, '
    'then come back and tap Enable push notifications again. Split Spends '
    'does not need Google services for this.';

const distributorsUrl = 'https://unifiedpush.org/users/distributors/';
