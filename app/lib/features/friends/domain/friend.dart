import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';

class Friend {
  const Friend({required this.user, this.balances = const []});

  factory Friend.fromJson(JsonMap json) => Friend(
    user: User.fromJson(json['user'] as JsonMap),
    balances: parseBalances(json['balances']),
  );

  final User user;

  /// Positive: the friend owes you.
  final List<Balance> balances;
}

class FriendGroupBalance {
  const FriendGroupBalance({
    required this.groupId,
    required this.groupName,
    this.balances = const [],
  });

  factory FriendGroupBalance.fromJson(JsonMap json) => FriendGroupBalance(
    groupId: json['groupId'] as String,
    groupName: (json['groupName'] as String?) ?? '',
    balances: parseBalances(json['balances']),
  );

  final String groupId;
  final String groupName;
  final List<Balance> balances;
}

/// `GET /friends/{userId}`.
class FriendDetail {
  const FriendDetail({
    required this.user,
    this.balances = const [],
    this.groups = const [],
    this.nonGroup = const [],
  });

  factory FriendDetail.fromJson(JsonMap json) => FriendDetail(
    user: User.fromJson(json['user'] as JsonMap),
    balances: parseBalances(json['balances']),
    groups: (json['groups'] as List? ?? const [])
        .cast<JsonMap>()
        .map(FriendGroupBalance.fromJson)
        .toList(),
    nonGroup: parseBalances(json['nonGroup']),
  );

  final User user;
  final List<Balance> balances;
  final List<FriendGroupBalance> groups;
  final List<Balance> nonGroup;
}
