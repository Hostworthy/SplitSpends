import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';

class LoginResult {
  const LoginResult({
    required this.token,
    required this.me,
    required this.isNew,
    this.expiresAt,
  });

  factory LoginResult.fromJson(JsonMap json) => LoginResult(
    token: json['token'] as String,
    me: Me.fromJson(json['user'] as JsonMap),
    isNew: (json['isNew'] as bool?) ?? false,
    expiresAt: json['expiresAt'] as String?,
  );

  final String token;
  final Me me;
  final bool isNew;
  final String? expiresAt;
}

/// Email OTP login against a server (connected mode). Takes the server explicitly because it is chosen on the
/// login screen before any session exists.
abstract class AuthRepository {
  /// Returns how many seconds the code stays valid.
  Future<int> requestCode(String serverUrl, String email);

  Future<LoginResult> verifyCode(String serverUrl, String email, String code);

  Future<Me> setName(String serverUrl, String token, String name);
}
