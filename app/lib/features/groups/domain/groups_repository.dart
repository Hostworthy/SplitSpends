import 'dart:typed_data';

import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/stats.dart';

/// Groups, their members and balances. Implemented on-device (local mode)
/// and over HTTP (connected mode).
abstract interface class GroupsRepository {
  Future<List<Group>> list({bool archived = false});

  Future<Group> get(String id);

  Future<Group> create(GroupInput input);

  Future<Group> update(String id, GroupInput input);

  /// Fails when the group is not settled.
  Future<void> delete(String id);

  /// Adds someone by email (connected) or by name (local, email optional).
  Future<Group> addMember(String id, String email, {String? name});

  /// Fails when that member is not settled.
  Future<void> removeMember(String id, String userId);

  Future<GroupBalances> balances(String id);

  /// CSV of all expenses and payments.
  Future<Uint8List> export(String id);

  /// Spending stats for a group, or for yourself when [groupId] is null.
  Future<Stats> stats({String? groupId, String? currency});
}
