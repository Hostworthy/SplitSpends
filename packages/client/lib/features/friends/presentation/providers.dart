import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/features/friends/data/providers.dart';
import 'package:splitspends/features/friends/domain/friend.dart';

final friendsProvider = FutureProvider<List<Friend>>(
  (ref) => ref.watch(friendsRepositoryProvider).list(),
);

final friendDetailProvider = FutureProvider.family<FriendDetail, String>(
  (ref, id) => ref.watch(friendsRepositoryProvider).get(id),
);
