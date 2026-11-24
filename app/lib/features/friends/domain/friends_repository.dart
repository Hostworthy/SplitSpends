import 'package:splitspends/features/friends/domain/friend.dart';

/// Friends and what you owe each other across groups.
abstract interface class FriendsRepository {
  Future<List<Friend>> list();

  /// Adds a friend by email; [name] is used for people not yet registered.
  Future<Friend> add(String email, {String? name});

  Future<FriendDetail> get(String userId);

  /// Fails when you are not settled up.
  Future<void> remove(String userId);
}
