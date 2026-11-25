import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/paging/paged.dart';
import 'package:splitspends/features/activity/data/providers.dart';
import 'package:splitspends/features/activity/domain/activity.dart';

/// Activity feed; the argument is a group id or `''` for everything.
final activityFeedProvider =
    AsyncNotifierProvider.family<ActivityFeed, Paged<Activity>, String>(
      ActivityFeed.new,
    );

class ActivityFeed extends PagedNotifier<Activity> {
  ActivityFeed(this.groupId);

  final String groupId;

  @override
  Future<PageResult<Activity>> fetch(int offset, int limit) => ref
      .read(activityRepositoryProvider)
      .list(
        groupId: groupId.isEmpty ? null : groupId,
        limit: limit,
        offset: offset,
      );

  @override
  Object key(Activity item) => item.id;

  @override
  Future<Paged<Activity>> build() {
    ref.watch(activityRepositoryProvider);
    return super.build();
  }
}
