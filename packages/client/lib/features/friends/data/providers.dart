import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/friends/data/local_friends_repository.dart';
import 'package:splitspends/features/friends/data/remote_friends_repository.dart';
import 'package:splitspends/features/friends/domain/friends_repository.dart';

final friendsRepositoryProvider = Provider<FriendsRepository>(
  (ref) => byMode(
    ref,
    remote: RemoteFriendsRepository.new,
    local: () => LocalFriendsRepository(ref.watch(localLedgerProvider)),
  ),
);
