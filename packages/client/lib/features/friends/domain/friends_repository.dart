import 'package:splitspends/features/friends/domain/friend.dart';

/// Friends and what you owe each other across groups.
abstract interface class FriendsRepository {
  Future<List<Friend>> list();

  /// Adds a friend by email, by name only (a placeholder), or both.
  Future<Friend> add({String? email, String? name});

  /// Gives a placeholder an email (and invites them when connected).
  Future<Friend> invite(String userId, String email);

  Future<FriendDetail> get(String userId);

  /// Fails when you are not settled up.
  Future<void> remove(String userId);
}
