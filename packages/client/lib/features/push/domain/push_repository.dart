import 'package:splitspends/features/push/domain/push_device.dart';

/// Devices registered for push on the server (connected mode only).
abstract interface class PushRepository {
  Future<List<PushDevice>> list();

  /// Registers this device, or refreshes it when the endpoint is known.
  Future<PushDevice> register(PushKeys keys);

  Future<void> remove(String id);

  /// Sends a test notification to every registered device.
  Future<PushTestResult> test();
}
