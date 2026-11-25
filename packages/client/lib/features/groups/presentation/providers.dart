import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/stats.dart';

final groupsProvider = FutureProvider<List<Group>>(
  (ref) => ref.watch(groupsRepositoryProvider).list(),
);

final archivedGroupsProvider = FutureProvider<List<Group>>(
  (ref) => ref.watch(groupsRepositoryProvider).list(archived: true),
);

final groupProvider = FutureProvider.family<Group, String>(
  (ref, id) => ref.watch(groupsRepositoryProvider).get(id),
);

final groupBalancesProvider = FutureProvider.family<GroupBalances, String>(
  (ref, id) => ref.watch(groupsRepositoryProvider).balances(id),
);

/// Stats for a group (or yourself when groupId is null) in one currency.
final statsProvider =
    FutureProvider.family<Stats, ({String? groupId, String? currency})>(
      (ref, key) => ref
          .watch(groupsRepositoryProvider)
          .stats(groupId: key.groupId, currency: key.currency),
    );
