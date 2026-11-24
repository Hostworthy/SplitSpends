import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';

enum GroupKind {
  home('Home', '🏠'),
  trip('Trip', '✈️'),
  couple('Couple', '💞'),
  other('Other', '👥');

  const GroupKind(this.label, this.emoji);

  final String label;
  final String emoji;

  static GroupKind parse(String? value) => GroupKind.values.firstWhere(
    (k) => k.name == value,
    orElse: () => GroupKind.other,
  );
}

class Group {
  const Group({
    required this.id,
    required this.name,
    this.kind = GroupKind.other,
    this.defaultCurrency = 'USD',
    this.simplifyDebts = true,
    this.emoji,
    this.color,
    this.archived = false,
    this.members = const [],
    this.balances = const [],
    this.createdAt,
    this.updatedAt,
  });

  factory Group.fromJson(JsonMap json) => Group(
    id: json['id'] as String,
    name: (json['name'] as String?) ?? '',
    kind: GroupKind.parse(json['kind'] as String?),
    defaultCurrency: (json['defaultCurrency'] as String?) ?? 'USD',
    simplifyDebts: (json['simplifyDebts'] as bool?) ?? true,
    emoji: json['emoji'] as String?,
    color: json['color'] as String?,
    archived: (json['archived'] as bool?) ?? false,
    members: (json['members'] as List? ?? const [])
        .cast<JsonMap>()
        .map(User.fromJson)
        .toList(),
    balances: parseBalances(json['balances']),
    createdAt: json['createdAt'] as String?,
    updatedAt: json['updatedAt'] as String?,
  );

  final String id;
  final String name;
  final GroupKind kind;
  final String defaultCurrency;
  final bool simplifyDebts;
  final String? emoji;
  final String? color;
  final bool archived;
  final List<User> members;
  final List<Balance> balances;
  final String? createdAt;
  final String? updatedAt;

  String get displayEmoji =>
      (emoji != null && emoji!.isNotEmpty) ? emoji! : kind.emoji;

  User? member(String id) {
    for (final m in members) {
      if (m.id == id) return m;
    }
    return null;
  }
}

/// Request body for creating or patching a group.
class GroupInput {
  const GroupInput({
    this.name,
    this.kind,
    this.defaultCurrency,
    this.simplifyDebts,
    this.emoji,
    this.color,
    this.archived,
    this.memberEmails,
  });

  final String? name;
  final GroupKind? kind;
  final String? defaultCurrency;
  final bool? simplifyDebts;
  final String? emoji;
  final String? color;
  final bool? archived;
  final List<String>? memberEmails;

  JsonMap toJson() => {
    'name': ?name,
    'kind': ?kind?.name,
    'defaultCurrency': ?defaultCurrency,
    'simplifyDebts': ?simplifyDebts,
    'emoji': ?emoji,
    'color': ?color,
    'archived': ?archived,
    if (memberEmails != null)
      'members': [
        for (final email in memberEmails!) {'email': email},
      ],
  };
}

class MemberBalance {
  const MemberBalance({required this.userId, required this.balances});

  factory MemberBalance.fromJson(JsonMap json) => MemberBalance(
    userId: json['userId'] as String,
    balances: parseBalances(json['balances']),
  );

  final String userId;
  final List<Balance> balances;
}

/// `GET /groups/{id}/balances`.
class GroupBalances {
  const GroupBalances({this.members = const [], this.debts = const []});

  factory GroupBalances.fromJson(JsonMap json) => GroupBalances(
    members: (json['members'] as List? ?? const [])
        .cast<JsonMap>()
        .map(MemberBalance.fromJson)
        .toList(),
    debts: (json['debts'] as List? ?? const [])
        .cast<JsonMap>()
        .map(Debt.fromJson)
        .toList(),
  );

  final List<MemberBalance> members;
  final List<Debt> debts;
}
