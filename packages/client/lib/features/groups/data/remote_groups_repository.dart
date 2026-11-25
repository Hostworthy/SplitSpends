import 'dart:typed_data';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/stats.dart';

/// HTTP implementation used in connected mode.
class RemoteGroupsRepository implements GroupsRepository {
  RemoteGroupsRepository(this._api);

  final ApiClient _api;

  @override
  Future<List<Group>> list({bool archived = false}) async =>
      (await _api.getList(
        '/groups',
        Group.fromJson,
        query: {'archived': archived ? 'true' : null},
      )).items;

  @override
  Future<Group> get(String id) =>
      _api.getData('/groups/$id', (d) => Group.fromJson(d as JsonMap));

  @override
  Future<Group> create(GroupInput input) => _api.postData(
    '/groups',
    input.toJson(),
    (d) => Group.fromJson(d as JsonMap),
  );

  @override
  Future<Group> update(String id, GroupInput input) => _api.patchData(
    '/groups/$id',
    input.toJson(),
    (d) => Group.fromJson(d as JsonMap),
  );

  @override
  Future<void> delete(String id) => _api.delete('/groups/$id');

  @override
  Future<Group> addMember(String id, {String? email, String? name}) =>
      _api.postData(
        '/groups/$id/members',
        MemberInput(email: email, name: name).toJson(),
        (d) => Group.fromJson(d as JsonMap),
      );

  @override
  Future<void> removeMember(String id, String userId) =>
      _api.delete('/groups/$id/members/$userId');

  @override
  Future<GroupBalances> balances(String id) => _api.getData(
    '/groups/$id/balances',
    (d) => GroupBalances.fromJson(d as JsonMap),
  );

  @override
  Future<Uint8List> export(String id) => _api.getBytes('/groups/$id/export');

  @override
  Future<Stats> stats({String? groupId, String? currency}) => _api.getData(
    '/stats',
    (d) => Stats.fromJson(d as JsonMap),
    query: {'groupId': groupId, 'currency': currency},
  );
}
