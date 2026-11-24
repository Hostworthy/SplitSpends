import 'dart:typed_data';

import 'package:uuid/uuid.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/domain/stats.dart';

/// Connected mode: queues group creation while offline (everything else
/// needs the server's answer, e.g. member invites).
class OfflineGroupsRepository implements GroupsRepository {
  OfflineGroupsRepository(
    this._remote,
    this._outbox, {
    required this.me,
    required this.onQueued,
  });

  final GroupsRepository _remote;
  final Outbox _outbox;
  final User? Function() me;
  final void Function() onQueued;

  @override
  Future<Group> create(GroupInput input) async {
    final name = input.name?.trim() ?? '';
    if (name.isEmpty) {
      throw const ApiError.validation('name', 'Give the group a name.');
    }
    final withId = GroupInput(
      id: input.id ?? const Uuid().v4(),
      name: name,
      kind: input.kind,
      defaultCurrency: input.defaultCurrency,
      simplifyDebts: input.simplifyDebts,
      emoji: input.emoji,
      color: input.color,
      members: input.members,
    );
    if ((await _outbox.pending()).isEmpty) {
      try {
        return await _remote.create(withId);
      } on ApiError catch (error) {
        if (!error.isNetwork) rethrow;
      }
    }
    await _outbox.add(
      'POST',
      '/groups',
      body: withId.toJson(),
      label: 'Create “$name”',
    );
    onQueued();
    final self = me();
    return Group(
      id: withId.id!,
      name: name,
      kind: withId.kind ?? GroupKind.other,
      defaultCurrency: withId.defaultCurrency ?? self?.defaultCurrency ?? 'USD',
      simplifyDebts: withId.simplifyDebts ?? true,
      emoji: withId.emoji,
      color: withId.color,
      members: [?self],
    );
  }

  @override
  Future<List<Group>> list({bool archived = false}) =>
      _remote.list(archived: archived);

  @override
  Future<Group> get(String id) => _remote.get(id);

  @override
  Future<Group> update(String id, GroupInput input) =>
      _remote.update(id, input);

  @override
  Future<void> delete(String id) => _remote.delete(id);

  @override
  Future<Group> addMember(String id, {String? email, String? name}) =>
      _remote.addMember(id, email: email, name: name);

  @override
  Future<void> removeMember(String id, String userId) =>
      _remote.removeMember(id, userId);

  @override
  Future<GroupBalances> balances(String id) => _remote.balances(id);

  @override
  Future<Uint8List> export(String id) => _remote.export(id);

  @override
  Future<Stats> stats({String? groupId, String? currency}) =>
      _remote.stats(groupId: groupId, currency: currency);
}
