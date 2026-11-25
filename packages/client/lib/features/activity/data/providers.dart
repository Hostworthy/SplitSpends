import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/activity/data/local_activity_repository.dart';
import 'package:splitspends/features/activity/data/remote_activity_repository.dart';
import 'package:splitspends/features/activity/domain/activity_repository.dart';

final activityRepositoryProvider = Provider<ActivityRepository>(
  (ref) => byMode(
    ref,
    remote: RemoteActivityRepository.new,
    local: () => LocalActivityRepository(ref.watch(localLedgerProvider)),
  ),
);
