import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/admin/domain/admin.dart';

class RemoteAdminRepository implements AdminRepository {
  RemoteAdminRepository(this._api);

  final ApiClient _api;

  @override
  Future<SignupSettings> signup() => _api.getData(
    '/admin/signup',
    (d) => SignupSettings.fromJson(d as JsonMap),
  );

  @override
  Future<SignupSettings> saveSignup(SignupSettings settings) => _api.putData(
    '/admin/signup',
    settings.toJson(),
    (d) => SignupSettings.fromJson(d as JsonMap),
  );

  /// The server keeps at most this many requests waiting, so reading up to
  /// it shows every pending request; decided ones stop at the newest.
  static const _most = 500;

  /// Reads page after page, so the oldest people waiting are listed too.
  @override
  Future<List<SignupRequest>> requests({String status = 'pending'}) async {
    final all = <SignupRequest>[];
    while (all.length < _most) {
      final page = await _api.getList(
        '/admin/requests',
        SignupRequest.fromJson,
        query: {'limit': 200, 'offset': all.length, 'status': status},
      );
      all.addAll(page.items);
      if (page.items.isEmpty || all.length >= page.total) break;
    }
    return all;
  }

  @override
  Future<SignupRequest> approve(String id) => _api.postData(
    '/admin/requests/$id/approve',
    null,
    (d) => SignupRequest.fromJson(d as JsonMap),
  );

  @override
  Future<SignupRequest> decline(String id) => _api.postData(
    '/admin/requests/$id/decline',
    null,
    (d) => SignupRequest.fromJson(d as JsonMap),
  );

  @override
  Future<List<Account>> accounts({
    String status = '',
    String query = '',
  }) async => (await _api.getList(
    '/admin/accounts',
    Account.fromJson,
    query: {
      'limit': 200,
      if (status.isNotEmpty) 'status': status,
      if (query.isNotEmpty) 'q': query,
    },
  )).items;

  @override
  Future<Account> setRole(String userId, String role) => _api.patchData(
    '/admin/accounts/$userId',
    {'role': role},
    (d) => Account.fromJson(d as JsonMap),
  );

  @override
  Future<Account> disable(String userId) => _api.postData(
    '/admin/accounts/$userId/disable',
    null,
    (d) => Account.fromJson(d as JsonMap),
  );

  @override
  Future<Account> enable(String userId) => _api.postData(
    '/admin/accounts/$userId/enable',
    null,
    (d) => Account.fromJson(d as JsonMap),
  );
}
