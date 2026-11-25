import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/core/models/json.dart';

class CurrencyInfo {
  const CurrencyInfo({
    required this.code,
    required this.name,
    required this.symbol,
    required this.exponent,
  });

  factory CurrencyInfo.fromJson(JsonMap json) => CurrencyInfo(
    code: json['code'] as String,
    name: (json['name'] as String?) ?? json['code'] as String,
    symbol: (json['symbol'] as String?) ?? json['code'] as String,
    exponent: (json['exponent'] as num?)?.toInt() ?? 2,
  );

  final String code;
  final String name;
  final String symbol;
  final int exponent;
}

class Category {
  const Category({
    required this.id,
    required this.name,
    required this.group,
    this.icon,
    this.custom = false,
    this.mine = false,
  });

  factory Category.fromJson(JsonMap json) => Category(
    id: json['id'] as String,
    name: (json['name'] as String?) ?? json['id'] as String,
    group: (json['group'] as String?) ?? 'Other',
    icon: json['icon'] as String?,
    custom: (json['custom'] as bool?) ?? false,
    mine: (json['mine'] as bool?) ?? false,
  );

  final String id;
  final String name;
  final String group;

  /// A Material Symbols name such as `restaurant`.
  final String? icon;

  /// User-defined (`custom.<uuid>`); [mine] when the caller owns it.
  final bool custom;
  final bool mine;
}

/// Public server configuration from `GET /config`.
class AppConfig {
  const AppConfig({
    this.appName = 'Split Spends',
    this.aiEnabled = false,
    this.signupMode = 'open',
    this.currencies = const [],
    this.categories = const [],
    this.pushEnabled = false,
    this.vapidPublicKey = '',
    this.gravatarEnabled = false,
  });

  factory AppConfig.fromJson(JsonMap json) => AppConfig(
    appName: (json['appName'] as String?) ?? 'Split Spends',
    aiEnabled: (json['aiEnabled'] as bool?) ?? false,
    signupMode: (json['signupMode'] as String?) ?? 'open',
    pushEnabled: (json['pushEnabled'] as bool?) ?? false,
    vapidPublicKey: (json['vapidPublicKey'] as String?) ?? '',
    gravatarEnabled: (json['gravatarEnabled'] as bool?) ?? false,
    currencies: (json['currencies'] as List? ?? const [])
        .cast<JsonMap>()
        .map(CurrencyInfo.fromJson)
        .toList(),
    categories: (json['categories'] as List? ?? const [])
        .cast<JsonMap>()
        .map(Category.fromJson)
        .toList(),
  );

  final String appName;
  final bool aiEnabled;
  final String signupMode;
  final List<CurrencyInfo> currencies;
  final List<Category> categories;

  /// True when the server sends Web Push notifications.
  final bool pushEnabled;

  /// The server's VAPID key (base64url) that devices subscribe with.
  final String vapidPublicKey;

  /// Whether people may show their Gravatar picture on this server.
  final bool gravatarEnabled;

  CurrencyInfo currency(String code) {
    for (final c in currencies) {
      if (c.code == code) return c;
    }
    return fallbackCurrency(code);
  }

  Category? category(String? id) {
    if (id == null) return null;
    for (final c in categories) {
      if (c.id == id) return c;
    }
    return null;
  }

  List<String> get currencyCodes => currencies.isEmpty
      ? fallbackCodes
      : currencies.map((c) => c.code).toList();
}

const fallbackCodes = [
  'USD',
  'EUR',
  'GBP',
  'INR',
  'JPY',
  'CAD',
  'AUD',
  'CHF',
  'CNY',
  'SGD',
];

/// [code] as the bundled catalog describes it; used when the server config
/// is not loaded yet.
CurrencyInfo fallbackCurrency(String code) {
  for (final c in localConfig.currencies) {
    if (c.code == code) return c;
  }
  return CurrencyInfo(code: code, name: code, symbol: code, exponent: 2);
}
