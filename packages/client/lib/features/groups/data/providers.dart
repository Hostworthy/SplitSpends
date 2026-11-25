import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/mode.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/groups/data/offline_groups_repository.dart';
import 'package:splitspends/features/groups/data/remote_groups_repository.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';

final groupsRepositoryProvider = Provider<GroupsRepository>(
  (ref) => byMode(
    ref,
    remote: (api) => OfflineGroupsRepository(
      RemoteGroupsRepository(api),
      ref.watch(outboxProvider),
      me: () => ref.read(meProvider),
    ),
    local: () => LocalGroupsRepository(ref.watch(localLedgerProvider)),
  ),
);
