import 'package:http/http.dart' as http;

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/invites/domain/invite.dart';

/// HTTP implementation used in connected mode.
class RemoteInvitesRepository implements InvitesRepository {
  RemoteInvitesRepository(this._api);

  final ApiClient _api;

  @override
  Future<Invite> create(String groupId) => _api.postData(
    '/groups/$groupId/invites',
    null,
    (d) => Invite.fromJson(d as JsonMap),
  );

  @override
  Future<List<Invite>> list(String groupId) async =>
      (await _api.getList('/groups/$groupId/invites', Invite.fromJson)).items;

  @override
  Future<void> revoke(String groupId, String inviteId) =>
      _api.delete('/groups/$groupId/invites/$inviteId');

  @override
  Future<Group> accept(String token) => _api.postData(
    '/invites/${Uri.encodeComponent(token)}/accept',
    null,
    (d) => Group.fromJson(d as JsonMap),
  );
}

/// Reads previews without a session.
class RemoteInvitePreviews implements InvitePreviews {
  RemoteInvitePreviews(this._http);

  final http.Client _http;

  @override
  Future<InvitePreview> preview(String serverUrl, String token) =>
      ApiClient(serverUrl: serverUrl, httpClient: _http).getData(
        '/invites/${Uri.encodeComponent(token)}',
        (d) => InvitePreview.fromJson(d as JsonMap),
      );
}
