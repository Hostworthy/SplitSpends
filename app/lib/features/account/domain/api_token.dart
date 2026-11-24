import 'package:splitspends/core/models/json.dart';

class ApiToken {
  const ApiToken({
    required this.id,
    required this.name,
    required this.createdAt,
    this.lastUsedAt,
    this.token,
  });

  factory ApiToken.fromJson(JsonMap json) => ApiToken(
    id: json['id'] as String,
    name: (json['name'] as String?) ?? '',
    createdAt: (json['createdAt'] as String?) ?? '',
    lastUsedAt: json['lastUsedAt'] as String?,
    token: json['token'] as String?,
  );

  final String id;
  final String name;
  final String createdAt;
  final String? lastUsedAt;

  /// Only present right after creation.
  final String? token;
}
