import 'package:splitspends/core/models/json.dart';

/// A device registered on the server for push notifications.
class PushDevice {
  const PushDevice({
    required this.id,
    required this.platform,
    required this.label,
    required this.endpoint,
    required this.createdAt,
    this.lastSuccessAt,
    this.failures = 0,
  });

  factory PushDevice.fromJson(JsonMap json) => PushDevice(
    id: json['id'] as String,
    platform: (json['platform'] as String?) ?? 'web',
    label: (json['label'] as String?) ?? '',
    endpoint: (json['endpoint'] as String?) ?? '',
    createdAt: (json['createdAt'] as String?) ?? '',
    lastSuccessAt: json['lastSuccessAt'] as String?,
    failures: (json['failures'] as num?)?.toInt() ?? 0,
  );

  final String id;

  /// `web` or `unifiedpush`.
  final String platform;
  final String label;
  final String endpoint;
  final String createdAt;
  final String? lastSuccessAt;
  final int failures;

  String get displayName => label.isNotEmpty
      ? label
      : (platform == 'unifiedpush' ? 'Android phone' : 'Browser');
}

/// What this device's push service handed out: where to send messages and
/// the keys to encrypt them with (base64url).
class PushKeys {
  const PushKeys({
    required this.endpoint,
    required this.p256dh,
    required this.auth,
    required this.platform,
    this.label = '',
  });

  factory PushKeys.fromJson(JsonMap json) => PushKeys(
    endpoint: json['endpoint'] as String,
    p256dh: (json['keys'] as JsonMap)['p256dh'] as String,
    auth: (json['keys'] as JsonMap)['auth'] as String,
    platform: (json['platform'] as String?) ?? 'web',
    label: (json['label'] as String?) ?? '',
  );

  final String endpoint;
  final String p256dh;
  final String auth;
  final String platform;
  final String label;

  /// The `POST /me/push-subscriptions` body.
  JsonMap toJson() => {
    'endpoint': endpoint,
    'keys': {'p256dh': p256dh, 'auth': auth},
    'platform': platform,
    'label': label,
  };
}

/// What a test notification reached.
class PushTestResult {
  const PushTestResult({this.sent = 0, this.failed = 0, this.removed = 0});

  factory PushTestResult.fromJson(JsonMap json) => PushTestResult(
    sent: (json['sent'] as num?)?.toInt() ?? 0,
    failed: (json['failed'] as num?)?.toInt() ?? 0,
    removed: (json['removed'] as num?)?.toInt() ?? 0,
  );

  final int sent;
  final int failed;
  final int removed;

  /// One line for a snackbar.
  String get summary {
    final parts = <String>[
      'Sent to $sent device${sent == 1 ? '' : 's'}',
      if (failed > 0) '$failed failed',
      if (removed > 0) '$removed no longer registered',
    ];
    return '${parts.join(', ')}.';
  }
}

/// A user-facing reason push could not be turned on.
class PushError implements Exception {
  const PushError(this.message, {this.needsDistributor = false});

  final String message;

  /// True on Android when no UnifiedPush distributor app is installed.
  final bool needsDistributor;

  @override
  String toString() => message;
}

/// A UnifiedPush distributor app installed on an Android phone.
class Distributor {
  const Distributor(this.id);

  /// The Android package name, for example `io.heckel.ntfy`.
  final String id;

  String get name => distributorName(id);
}

const _knownDistributors = {
  'io.heckel.ntfy': 'ntfy',
  'org.unifiedpush.distributor.nextpush': 'NextPush',
  'org.unifiedpush.distributor.fcm': 'FCM Distributor',
  'org.unifiedpush.distributor.sunup': 'Sunup',
  'org.unifiedpush.distributor.conversations': 'Conversations',
  'eu.siacs.conversations': 'Conversations',
  'im.molly.app': 'Molly',
};

/// A readable name for a distributor package.
String distributorName(String id) {
  final known = _knownDistributors[id];
  if (known != null) return known;
  final last = id.split('.').where((p) => p.isNotEmpty).lastOrNull ?? id;
  return last.isEmpty ? id : last[0].toUpperCase() + last.substring(1);
}

/// Names a browser from its user agent, for example "Firefox on Linux".
String browserLabel(String userAgent) {
  String? pick(List<(String, String)> options) {
    for (final (token, name) in options) {
      if (userAgent.contains(token)) return name;
    }
    return null;
  }

  final browser = pick(const [
    ('Edg/', 'Edge'),
    ('OPR/', 'Opera'),
    ('Firefox/', 'Firefox'),
    ('Chrome/', 'Chrome'),
    ('Safari/', 'Safari'),
  ]);
  final system = pick(const [
    ('Android', 'Android'),
    ('iPhone', 'iPhone'),
    ('iPad', 'iPad'),
    ('Windows', 'Windows'),
    ('Mac OS', 'macOS'),
    ('CrOS', 'ChromeOS'),
    ('Linux', 'Linux'),
  ]);
  if (browser != null && system != null) return '$browser on $system';
  return browser ?? system ?? 'Browser';
}
