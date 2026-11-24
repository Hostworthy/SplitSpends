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
  });

  factory Category.fromJson(JsonMap json) => Category(
    id: json['id'] as String,
    name: (json['name'] as String?) ?? json['id'] as String,
    group: (json['group'] as String?) ?? 'Other',
    icon: json['icon'] as String?,
  );

  final String id;
  final String name;
  final String group;
  final String? icon;
}

/// Public server configuration from `GET /config`.
class AppConfig {
  const AppConfig({
    this.appName = 'SplitSpends',
    this.aiEnabled = false,
    this.signupMode = 'open',
    this.currencies = const [],
    this.categories = const [],
  });

  factory AppConfig.fromJson(JsonMap json) => AppConfig(
    appName: (json['appName'] as String?) ?? 'SplitSpends',
    aiEnabled: (json['aiEnabled'] as bool?) ?? false,
    signupMode: (json['signupMode'] as String?) ?? 'open',
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

/// Exponents for common currencies that do not use 2 decimals; used when the
/// server config is not loaded yet.
const _exponents = {
  'JPY': 0,
  'KRW': 0,
  'VND': 0,
  'CLP': 0,
  'ISK': 0,
  'HUF': 2,
  'BHD': 3,
  'KWD': 3,
  'OMR': 3,
  'JOD': 3,
  'TND': 3,
};

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

CurrencyInfo fallbackCurrency(String code) => CurrencyInfo(
  code: code,
  name: code,
  symbol: code,
  exponent: _exponents[code] ?? 2,
);
