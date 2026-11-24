import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/features/activity/domain/activity.dart';

/// The activity feed, newest first.
abstract interface class ActivityRepository {
  Future<PageResult<Activity>> list({
    String? groupId,
    String? expenseId,
    int limit = 30,
    int offset = 0,
  });
}
