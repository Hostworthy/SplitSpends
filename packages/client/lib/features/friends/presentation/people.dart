import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

/// Everyone the signed-in user knows (friends, group members, self), by id.
final peopleProvider = Provider<Map<String, User>>((ref) {
  final people = <String, User>{};
  for (final group in ref.watch(groupsProvider).value ?? const []) {
    for (final member in group.members) {
      people[member.id] = member;
    }
  }
  for (final friend in ref.watch(friendsProvider).value ?? const []) {
    people[friend.user.id] = friend.user;
  }
  final me = ref.watch(meProvider);
  if (me != null) people[me.id] = me;
  return people;
});

extension PeopleNames on Map<String, User> {
  /// "You" for the signed-in user, first name for others.
  String nameOf(String id, {String? meId, bool full = false}) {
    if (id == meId) return 'You';
    final user = this[id];
    if (user == null) return 'Someone';
    return full ? user.displayName : user.firstName;
  }
}
