import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';

class Activity {
  const Activity({
    required this.id,
    required this.kind,
    required this.actor,
    required this.summary,
    required this.createdAt,
    this.groupId,
    this.expenseId,
    this.data = const {},
  });

  factory Activity.fromJson(JsonMap json) => Activity(
    id: json['id'] as String,
    kind: (json['kind'] as String?) ?? '',
    actor: User.fromJson(json['actor'] as JsonMap),
    groupId: json['groupId'] as String?,
    expenseId: json['expenseId'] as String?,
    summary: (json['summary'] as String?) ?? '',
    data: (json['data'] as JsonMap?) ?? const {},
    createdAt: (json['createdAt'] as String?) ?? '',
  );

  final String id;
  final String kind;
  final User actor;
  final String? groupId;
  final String? expenseId;
  final String summary;
  final JsonMap data;
  final String createdAt;

  DateTime get time =>
      DateTime.tryParse(createdAt)?.toLocal() ?? DateTime.now();
}
