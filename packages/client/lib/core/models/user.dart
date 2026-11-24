import 'package:splitspends/core/models/json.dart';

/// A person in SplitSpends (yourself, a friend or a group member).
class User {
  const User({
    required this.id,
    this.email,
    required this.name,
    this.defaultCurrency = 'USD',
    this.avatarColor,
    this.status = 'active',
  });

  factory User.fromJson(JsonMap json) => User(
    id: json['id'] as String,
    email: _nonEmpty(json['email'] as String?),
    name: (json['name'] as String?) ?? '',
    defaultCurrency: (json['defaultCurrency'] as String?) ?? 'USD',
    avatarColor: json['avatarColor'] as String?,
    status: (json['status'] as String?) ?? 'active',
  );

  final String id;

  /// Null for people added by name only (placeholders).
  final String? email;
  final String name;
  final String defaultCurrency;
  final String? avatarColor;
  final String status;

  bool get isInvited => status == 'invited';
  bool get isPlaceholder => status == 'placeholder';

  /// Email, or a short note for people without one.
  String get contact => email ?? (isPlaceholder ? 'No email yet' : '');

  /// One line about the person's account state.
  String get statusLine => switch (status) {
    'invited' => '${email ?? ''} · invited',
    'placeholder' => email == null ? 'Added by name' : '$email · not joined',
    _ => email ?? '',
  };

  /// Name to show; falls back to the email's local part.
  String get displayName {
    if (name.trim().isNotEmpty) return name.trim();
    final mail = email ?? '';
    final at = mail.indexOf('@');
    if (at > 0) return mail.substring(0, at);
    return mail.isEmpty ? 'Someone' : mail;
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

  User copyWith({String? name, String? email, String? status}) => User(
    id: id,
    email: email ?? this.email,
    name: name ?? this.name,
    defaultCurrency: defaultCurrency,
    avatarColor: avatarColor,
    status: status ?? this.status,
  );

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
    super.email,
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

  @override
  Me copyWith({
    String? name,
    String? email,
    String? status,
    String? defaultCurrency,
    String? avatarColor,
    String? timezone,
    NotificationPrefs? notifications,
  }) => Me(
    id: id,
    email: email ?? this.email,
    name: name ?? this.name,
    defaultCurrency: defaultCurrency ?? this.defaultCurrency,
    avatarColor: avatarColor ?? this.avatarColor,
    status: status ?? this.status,
    timezone: timezone ?? this.timezone,
    notifications: notifications ?? this.notifications,
  );
}

String? _nonEmpty(String? value) =>
    value == null || value.trim().isEmpty ? null : value;
