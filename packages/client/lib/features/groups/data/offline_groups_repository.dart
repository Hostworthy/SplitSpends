import 'dart:typed_data';

import 'package:uuid/uuid.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/domain/stats.dart';

/// Connected mode: queues group creation while offline (everything else
/// needs the server's answer, e.g. member invites). Groups still waiting to
/// be created are shown alongside the server's.
class OfflineGroupsRepository implements GroupsRepository {
  OfflineGroupsRepository(this._remote, this._outbox, {required this.me});

  final GroupsRepository _remote;
  final Outbox _outbox;
  final User? Function() me;

  @override
  Future<Group> create(GroupInput input) async {
    final name = input.name?.trim() ?? '';
    if (name.isEmpty) {
      throw const ApiError.validation('name', 'Give the group a name.');
    }
    final withId = input.copyWith(
      id: input.id ?? const Uuid().v4(),
      name: name,
    );
    final body = withId.toJson();
    return _outbox.write(
      online: () => _remote.create(withId),
      offline: () => _optimistic(body),
      method: 'POST',
      path: '/groups',
      body: body,
      label: 'Create “$name”',
    );
  }

  @override
  Future<List<Group>> list({bool archived = false}) async {
    // Read the queue first: a create sent meanwhile is then in both, and
    // shown once, rather than in neither.
    final pending = await _pending();
    final groups = await _remote.list(archived: archived);
    if (archived || pending.isEmpty) return groups;
    final listed = {for (final g in groups) g.id};
    return [
      // Newest first, as the server orders them.
      for (final g in pending.reversed)
        if (!listed.contains(g.id)) g,
      ...groups,
    ];
  }

  @override
  Future<Group> get(String id) async {
    for (final g in await _pending()) {
      if (g.id == id) return g;
    }
    return _remote.get(id);
  }

  /// Groups the queued writes create, oldest first.
  Future<List<Group>> _pending() async => [
    for (final op in await _outbox.pending())
      if (op.method == 'POST' && op.path == '/groups' && op.body is Map)
        _optimistic((op.body! as Map).cast<String, dynamic>()),
  ];

  /// The group the server will create from a queued [body].
  Group _optimistic(JsonMap body) {
    final self = me();
    return Group(
      id: body['id'] as String,
      name: (body['name'] as String?) ?? '',
      kind: GroupKind.parse(body['kind'] as String?),
      defaultCurrency:
          (body['defaultCurrency'] as String?) ??
          self?.defaultCurrency ??
          'USD',
      simplifyDebts: (body['simplifyDebts'] as bool?) ?? true,
      emoji: body['emoji'] as String?,
      color: body['color'] as String?,
      defaultSplit: DefaultSplit.parse(body['defaultSplit']),
      members: [?self],
    );
  }

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
