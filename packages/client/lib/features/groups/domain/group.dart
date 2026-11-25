import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

/// What new expenses in a group start with (docs/API.md, Group). [values]
/// maps user ids to decimal text: empty for equal, every member for percent
/// and shares (`0` leaves someone out).
class DefaultSplit {
  const DefaultSplit({required this.method, this.values = const {}});

  static DefaultSplit? parse(Object? json) {
    if (json is! Map) return null;
    return DefaultSplit(
      method: SplitMethod.parse(json['method'] as String?),
      values: {
        for (final e in ((json['values'] as Map?) ?? const {}).entries)
          e.key as String: '${e.value}',
      },
    );
  }

  /// Methods a default may use (they do not depend on the total).
  static const methods = [
    SplitMethod.equal,
    SplitMethod.percent,
    SplitMethod.shares,
  ];

  final SplitMethod method;
  final Map<String, String> values;

  JsonMap toJson() => {'method': method.wire, 'values': values};
}

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
    this.defaultSplit,
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
    defaultSplit: DefaultSplit.parse(json['defaultSplit']),
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

  /// Null when there is none or it no longer matches the members.
  final DefaultSplit? defaultSplit;

  String get displayEmoji =>
      (emoji != null && emoji!.isNotEmpty) ? emoji! : kind.emoji;

  User? member(String id) {
    for (final m in members) {
      if (m.id == id) return m;
    }
    return null;
  }
}

/// Someone to add to a group: an email (invites them when connected), a
/// name (a placeholder you can invite later), or both.
class MemberInput {
  const MemberInput({this.email, this.name});

  final String? email;
  final String? name;

  /// Short label for chips.
  String get label => (name?.isNotEmpty ?? false) ? name! : (email ?? '');

  JsonMap toJson() => {
    if (email != null && email!.isNotEmpty) 'email': email,
    if (name != null && name!.isNotEmpty) 'name': name,
  };
}

/// Request body for creating or patching a group.
class GroupInput {
  const GroupInput({
    this.id,
    this.name,
    this.kind,
    this.defaultCurrency,
    this.simplifyDebts,
    this.emoji,
    this.color,
    this.archived,
    this.members,
    this.defaultSplit,
    this.clearDefaultSplit = false,
  });

  /// Client-chosen id for creates (idempotent retries).
  final String? id;
  final String? name;
  final GroupKind? kind;
  final String? defaultCurrency;
  final bool? simplifyDebts;
  final String? emoji;
  final String? color;
  final bool? archived;
  final List<MemberInput>? members;

  /// Sets the default split; [clearDefaultSplit] removes it instead.
  final DefaultSplit? defaultSplit;
  final bool clearDefaultSplit;

  GroupInput copyWith({String? id, String? name}) => GroupInput(
    id: id ?? this.id,
    name: name ?? this.name,
    kind: kind,
    defaultCurrency: defaultCurrency,
    simplifyDebts: simplifyDebts,
    emoji: emoji,
    color: color,
    archived: archived,
    members: members,
    defaultSplit: defaultSplit,
    clearDefaultSplit: clearDefaultSplit,
  );

  JsonMap toJson() => {
    'id': ?id,
    'name': ?name,
    'kind': ?kind?.name,
    'defaultCurrency': ?defaultCurrency,
    'simplifyDebts': ?simplifyDebts,
    'emoji': ?emoji,
    'color': ?color,
    'archived': ?archived,
    if (members != null) 'members': [for (final m in members!) m.toJson()],
    if (clearDefaultSplit)
      'defaultSplit': null
    else if (defaultSplit != null)
      'defaultSplit': defaultSplit!.toJson(),
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
