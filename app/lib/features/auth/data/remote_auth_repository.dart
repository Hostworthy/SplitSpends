import 'package:http/http.dart' as http;

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';

class RemoteAuthRepository implements AuthRepository {
  RemoteAuthRepository(this._http);

  final http.Client _http;

  ApiClient _client(String serverUrl, [String? token]) =>
      ApiClient(serverUrl: serverUrl, token: token, httpClient: _http);

  @override
  Future<int> requestCode(String serverUrl, String email) => _client(serverUrl)
      .postData('/auth/otp/request', {
        'email': email,
      }, (d) => ((d as JsonMap?)?['expiresInSeconds'] as num?)?.toInt() ?? 600);

  @override
  Future<LoginResult> verifyCode(String serverUrl, String email, String code) =>
      _client(serverUrl).postData('/auth/otp/verify', {
        'email': email,
        'code': code,
      }, (d) => LoginResult.fromJson(d as JsonMap));

  @override
  Future<Me> setName(String serverUrl, String token, String name) => _client(
    serverUrl,
    token,
  ).patchData('/me', {'name': name}, (d) => Me.fromJson(d as JsonMap));
}
