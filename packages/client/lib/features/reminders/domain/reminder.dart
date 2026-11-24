import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/json.dart';

/// A sent settle-up reminder (`POST /reminders`).
class Reminder {
  const Reminder({
    required this.id,
    required this.userId,
    this.groupId,
    this.balances = const [],
    this.emailed = true,
    this.sentAt,
  });

  factory Reminder.fromJson(JsonMap json) => Reminder(
    id: json['id'] as String,
    userId: json['userId'] as String,
    groupId: json['groupId'] as String?,
    balances: parseBalances(json['balances']),
    emailed: (json['emailed'] as bool?) ?? true,
    sentAt: json['sentAt'] as String?,
  );

  final String id;
  final String userId;
  final String? groupId;

  /// What the person owes you, per currency (positive).
  final List<Balance> balances;

  /// False when the person turned reminder emails off.
  final bool emailed;
  final String? sentAt;
}

/// Reminders need a server, which emails them. On-device users share
/// [reminderText] themselves instead.
abstract interface class RemindersRepository {
  /// Reminds [userId] what they owe you, in [groupId] or overall.
  Future<Reminder> send(String userId, {String? groupId});
}

/// Joins formatted amounts: `€12.50`, `€12.50 and $5.00`,
/// `€1.00, £2.00 and $3.00`.
String joinAmounts(List<String> amounts) => switch (amounts.length) {
  0 => '',
  1 => amounts.first,
  _ =>
    '${amounts.sublist(0, amounts.length - 1).join(', ')} and ${amounts.last}',
};

/// A friendly reminder the user can send through any messenger when there
/// is no server to email it (on-device mode).
String reminderText({
  required String name,
  required List<String> amounts,
  String? group,
}) {
  final where = group == null || group.isEmpty ? '' : ' for $group';
  return 'Hi $name, a friendly reminder that you owe me '
      '${joinAmounts(amounts)}$where. '
      'Could you settle up when you get a chance? Thanks!';
}
