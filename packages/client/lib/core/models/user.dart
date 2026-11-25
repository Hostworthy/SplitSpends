import 'package:characters/characters.dart';

import 'package:splitspends/core/models/json.dart';

/// A person in SplitSpends (yourself, a friend or a group member).
class User {
  const User({
    required this.id,
    this.email,
    required this.name,
    this.defaultCurrency = 'USD',
    this.avatarColor,
    this.avatarUrl,
    this.status = 'active',
  });

  factory User.fromJson(JsonMap json) => User(
    id: json['id'] as String,
    email: _nonEmpty(json['email'] as String?),
    name: (json['name'] as String?) ?? '',
    defaultCurrency: (json['defaultCurrency'] as String?) ?? 'USD',
    avatarColor: json['avatarColor'] as String?,
    avatarUrl: _nonEmpty(json['avatarUrl'] as String?),
    status: (json['status'] as String?) ?? 'active',
  );

  final String id;

  /// Null for people added by name only (placeholders).
  final String? email;
  final String name;
  final String defaultCurrency;
  final String? avatarColor;

  /// Gravatar picture when the person opted in; it may answer 404, so the
  /// initials stay the fallback.
  final String? avatarUrl;
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
    // Whole characters, so an emoji or accented letter is never cut in half.
    if (parts.length == 1) return parts.first.characters.first.toUpperCase();
    return (parts.first.characters.first + parts.last.characters.first)
        .toUpperCase();
  }

  User copyWith({String? name, String? email, String? status}) => User(
    id: id,
    email: email ?? this.email,
    name: name ?? this.name,
    defaultCurrency: defaultCurrency,
    avatarColor: avatarColor,
    avatarUrl: avatarUrl,
    status: status ?? this.status,
  );

  JsonMap toJson() => {
    'id': id,
    'email': email,
    'name': name,
    'defaultCurrency': defaultCurrency,
    'avatarColor': avatarColor,
    'avatarUrl': avatarUrl,
    'status': status,
  };
}

/// Things the server notifies about. [key] is the preference key in the API.
enum NotificationKind {
  expenseAdded('Expense added'),
  expenseUpdated('Expense edited or deleted'),
  paymentReceived('Payment received'),
  addedToGroup('Added to a group'),
  reminders('Settle-up reminders'),

  /// Only administrators and managers are told about these.
  approvals('People waiting to join');

  const NotificationKind(this.label);

  final String label;

  String get key => name;
}

/// Per-kind switches for one channel (email or push).
class ChannelPrefs {
  const ChannelPrefs({
    this.expenseAdded = true,
    this.expenseUpdated = true,
    this.paymentReceived = true,
    this.addedToGroup = true,
    this.reminders = true,
    this.approvals = true,
  });

  factory ChannelPrefs.fromJson(JsonMap? json) => ChannelPrefs(
    expenseAdded: (json?['expenseAdded'] as bool?) ?? true,
    expenseUpdated: (json?['expenseUpdated'] as bool?) ?? true,
    paymentReceived: (json?['paymentReceived'] as bool?) ?? true,
    addedToGroup: (json?['addedToGroup'] as bool?) ?? true,
    reminders: (json?['reminders'] as bool?) ?? true,
    approvals: (json?['approvals'] as bool?) ?? true,
  );

  final bool expenseAdded;
  final bool expenseUpdated;
  final bool paymentReceived;
  final bool addedToGroup;

  /// Settle-up reminders from people you owe.
  final bool reminders;

  /// People waiting to join the server.
  final bool approvals;

  bool on(NotificationKind kind) => switch (kind) {
    NotificationKind.expenseAdded => expenseAdded,
    NotificationKind.expenseUpdated => expenseUpdated,
    NotificationKind.paymentReceived => paymentReceived,
    NotificationKind.addedToGroup => addedToGroup,
    NotificationKind.reminders => reminders,
    NotificationKind.approvals => approvals,
  };

  /// A copy with [kind] switched to [value].
  ChannelPrefs set(NotificationKind kind, bool value) => ChannelPrefs(
    expenseAdded: kind == NotificationKind.expenseAdded ? value : expenseAdded,
    expenseUpdated: kind == NotificationKind.expenseUpdated
        ? value
        : expenseUpdated,
    paymentReceived: kind == NotificationKind.paymentReceived
        ? value
        : paymentReceived,
    addedToGroup: kind == NotificationKind.addedToGroup ? value : addedToGroup,
    reminders: kind == NotificationKind.reminders ? value : reminders,
    approvals: kind == NotificationKind.approvals ? value : approvals,
  );

  JsonMap toJson() => {for (final k in NotificationKind.values) k.key: on(k)};
}

/// Notification preferences for the signed-in user. In the API the email
/// switches sit at the top level of `notifications` and push is nested.
class NotificationPrefs {
  const NotificationPrefs({
    this.email = const ChannelPrefs(),
    this.push = const ChannelPrefs(),
  });

  factory NotificationPrefs.fromJson(JsonMap? json) => NotificationPrefs(
    email: ChannelPrefs.fromJson(json),
    push: ChannelPrefs.fromJson(json?['push'] as JsonMap?),
  );

  final ChannelPrefs email;
  final ChannelPrefs push;

  NotificationPrefs copyWith({ChannelPrefs? email, ChannelPrefs? push}) =>
      NotificationPrefs(email: email ?? this.email, push: push ?? this.push);

  JsonMap toJson() => {...email.toJson(), 'push': push.toJson()};
}

/// The signed-in user with private settings.
class Me extends User {
  const Me({
    required super.id,
    super.email,
    required super.name,
    super.defaultCurrency,
    super.avatarColor,
    super.avatarUrl,
    super.status,
    this.timezone,
    this.notifications = const NotificationPrefs(),
    this.gravatar = false,
    this.role = 'user',
  });

  factory Me.fromJson(JsonMap json) {
    final user = User.fromJson(json);
    return Me(
      id: user.id,
      email: user.email,
      name: user.name,
      defaultCurrency: user.defaultCurrency,
      avatarColor: user.avatarColor,
      avatarUrl: user.avatarUrl,
      status: user.status,
      timezone: json['timezone'] as String?,
      gravatar: (json['gravatar'] as bool?) ?? false,
      role: (json['role'] as String?) ?? 'user',
      notifications: NotificationPrefs.fromJson(
        json['notifications'] as JsonMap?,
      ),
    );
  }

  final String? timezone;
  final NotificationPrefs notifications;

  /// Whether you show your Gravatar picture (server accounts only).
  final bool gravatar;

  /// user, manager or admin.
  final String role;

  bool get isAdmin => role == 'admin';

  /// Administrators and managers approve people waiting to join.
  bool get canApprove => role == 'admin' || role == 'manager';

  /// The notification kinds this person can receive, so the ones they can
  /// set: approvals only reach administrators and managers.
  List<NotificationKind> get notificationKinds => [
    for (final kind in NotificationKind.values)
      if (canApprove || kind != NotificationKind.approvals) kind,
  ];

  @override
  Me copyWith({
    String? name,
    String? email,
    String? status,
    String? defaultCurrency,
    String? avatarColor,
    String? timezone,
    NotificationPrefs? notifications,
    bool? gravatar,
  }) => Me(
    id: id,
    email: email ?? this.email,
    name: name ?? this.name,
    defaultCurrency: defaultCurrency ?? this.defaultCurrency,
    avatarColor: avatarColor ?? this.avatarColor,
    avatarUrl: avatarUrl,
    status: status ?? this.status,
    timezone: timezone ?? this.timezone,
    notifications: notifications ?? this.notifications,
    gravatar: gravatar ?? this.gravatar,
    role: role,
  );
}

String? _nonEmpty(String? value) =>
    value == null || value.trim().isEmpty ? null : value;
