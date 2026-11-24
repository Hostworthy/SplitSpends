import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/activity/domain/activity_repository.dart';
import 'package:splitspends/features/activity/domain/activity.dart';

/// HTTP implementation used in connected mode.
class RemoteActivityRepository implements ActivityRepository {
  RemoteActivityRepository(this._api);

  final ApiClient _api;

  @override
  Future<PageResult<Activity>> list({
    String? groupId,
    String? expenseId,
    int limit = 30,
    int offset = 0,
  }) => _api.getList(
    '/activity',
    Activity.fromJson,
    query: {
      'groupId': groupId,
      'expenseId': expenseId,
      'limit': limit,
      'offset': offset,
    },
  );
}
