import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/reminders/domain/reminder.dart';

/// HTTP implementation used in connected mode.
class RemoteRemindersRepository implements RemindersRepository {
  RemoteRemindersRepository(this._api);

  final ApiClient _api;

  @override
  Future<Reminder> send(String userId, {String? groupId}) => _api.postData(
    '/reminders',
    {'userId': userId, 'groupId': ?groupId},
    (d) => Reminder.fromJson(d as JsonMap),
  );
}
