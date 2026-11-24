import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/friends/domain/friends_repository.dart';
import 'package:splitspends/features/friends/domain/friend.dart';

/// HTTP implementation used in connected mode.
class RemoteFriendsRepository implements FriendsRepository {
  RemoteFriendsRepository(this._api);

  final ApiClient _api;

  @override
  Future<List<Friend>> list() async => (await _api.getList(
    '/friends',
    Friend.fromJson,
    query: {'limit': 200},
  )).items;

  @override
  Future<Friend> add(String email, {String? name}) => _api.postData(
    '/friends',
    {'email': email, 'name': ?name},
    (d) => Friend.fromJson(d as JsonMap),
  );

  @override
  Future<FriendDetail> get(String userId) => _api.getData(
    '/friends/$userId',
    (d) => FriendDetail.fromJson(d as JsonMap),
  );

  @override
  Future<void> remove(String userId) => _api.delete('/friends/$userId');
}
