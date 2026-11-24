import 'package:splitspends/core/models/json.dart';

/// A person in SplitSpends (yourself, a friend or a group member).
class User {
  const User({
    required this.id,
    required this.email,
    required this.name,
    this.defaultCurrency = 'USD',
    this.avatarColor,
    this.status = 'active',
  });

  factory User.fromJson(JsonMap json) => User(
    id: json['id'] as String,
    email: (json['email'] as String?) ?? '',
    name: (json['name'] as String?) ?? '',
    defaultCurrency: (json['defaultCurrency'] as String?) ?? 'USD',
    avatarColor: json['avatarColor'] as String?,
    status: (json['status'] as String?) ?? 'active',
  );

  final String id;
  final String email;
  final String name;
  final String defaultCurrency;
  final String? avatarColor;
  final String status;

  bool get isInvited => status == 'invited';

  /// Name to show; falls back to the email's local part.
  String get displayName {
    if (name.trim().isNotEmpty) return name.trim();
    final at = email.indexOf('@');
    return at > 0 ? email.substring(0, at) : email;
  }

  String get firstName => displayName.split(' ').first;

  String get initials {
    final parts = displayName
        .split(RegExp(r'\s+'))
        .where((p) => p.isNotEmpty)
        .toList();
    if (parts.isEmpty) return '?';
    if (parts.length == 1) return parts.first.substring(0, 1).toUpperCase();
    return (parts.first.substring(0, 1) + parts.last.substring(0, 1))
        .toUpperCase();
  }

  JsonMap toJson() => {
    'id': id,
    'email': email,
    'name': name,
    'defaultCurrency': defaultCurrency,
    'avatarColor': avatarColor,
    'status': status,
  };
}

/// Notification preferences for the signed-in user.
class NotificationPrefs {
  const NotificationPrefs({
    this.expenseAdded = true,
    this.expenseUpdated = true,
    this.paymentReceived = true,
    this.addedToGroup = true,
  });

  factory NotificationPrefs.fromJson(JsonMap? json) => NotificationPrefs(
    expenseAdded: (json?['expenseAdded'] as bool?) ?? true,
    expenseUpdated: (json?['expenseUpdated'] as bool?) ?? true,
    paymentReceived: (json?['paymentReceived'] as bool?) ?? true,
    addedToGroup: (json?['addedToGroup'] as bool?) ?? true,
  );

  final bool expenseAdded;
  final bool expenseUpdated;
  final bool paymentReceived;
  final bool addedToGroup;

  NotificationPrefs copyWith({
    bool? expenseAdded,
    bool? expenseUpdated,
    bool? paymentReceived,
    bool? addedToGroup,
  }) => NotificationPrefs(
    expenseAdded: expenseAdded ?? this.expenseAdded,
    expenseUpdated: expenseUpdated ?? this.expenseUpdated,
    paymentReceived: paymentReceived ?? this.paymentReceived,
    addedToGroup: addedToGroup ?? this.addedToGroup,
  );

  JsonMap toJson() => {
    'expenseAdded': expenseAdded,
    'expenseUpdated': expenseUpdated,
    'paymentReceived': paymentReceived,
    'addedToGroup': addedToGroup,
  };
}

/// The signed-in user with private settings.
class Me extends User {
  const Me({
    required super.id,
    required super.email,
    required super.name,
    super.defaultCurrency,
    super.avatarColor,
    super.status,
    this.timezone,
    this.notifications = const NotificationPrefs(),
  });

  factory Me.fromJson(JsonMap json) {
    final user = User.fromJson(json);
    return Me(
      id: user.id,
      email: user.email,
      name: user.name,
      defaultCurrency: user.defaultCurrency,
      avatarColor: user.avatarColor,
      status: user.status,
      timezone: json['timezone'] as String?,
      notifications: NotificationPrefs.fromJson(
        json['notifications'] as JsonMap?,
      ),
    );
  }

  final String? timezone;
  final NotificationPrefs notifications;

  Me copyWith({
    String? name,
    String? defaultCurrency,
    String? avatarColor,
    String? timezone,
    NotificationPrefs? notifications,
  }) => Me(
    id: id,
    email: email,
    name: name ?? this.name,
    defaultCurrency: defaultCurrency ?? this.defaultCurrency,
    avatarColor: avatarColor ?? this.avatarColor,
    status: status,
    timezone: timezone ?? this.timezone,
    notifications: notifications ?? this.notifications,
  );
}
