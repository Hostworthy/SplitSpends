import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/push/domain/push_device.dart';
import 'package:splitspends/features/push/domain/push_repository.dart';

class RemotePushRepository implements PushRepository {
  RemotePushRepository(this._api);

  final ApiClient _api;

  @override
  Future<List<PushDevice>> list() async =>
      (await _api.getList('/me/push-subscriptions', PushDevice.fromJson)).items;

  @override
  Future<PushDevice> register(PushKeys keys) => _api.postData(
    '/me/push-subscriptions',
    keys.toJson(),
    (d) => PushDevice.fromJson(d as JsonMap),
  );

  @override
  Future<void> remove(String id) => _api.delete('/me/push-subscriptions/$id');

  @override
  Future<PushTestResult> test() => _api.postData(
    '/me/push-subscriptions/test',
    const <String, Object?>{},
    (d) => PushTestResult.fromJson(d as JsonMap),
  );
}
