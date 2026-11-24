import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:splitspends_ledger/ledger.dart';

import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/money/money.dart';

/// Where on-device mode downloads rates. Free, no key, about 160 currencies;
/// only the rate list is downloaded and nothing about the user is sent.
const publicRatesUrl = 'https://open.er-api.com/v6/latest/USD';

const _cacheKey = 'rates:public';

/// Today's exchange rates, or null when they are off or unavailable. In
/// connected mode they come from the server (which caches them); on this
/// device they come from [publicRatesUrl], cached for offline use.
final ratesProvider = FutureProvider<RateTable?>((ref) async {
  final mode = ref.watch(appModeProvider);
  final config = ref.watch(configValueProvider);
  final exponents = {for (final c in config.currencies) c.code: c.exponent};
  try {
    if (mode == AppMode.connected) {
      final client = ref.watch(apiClientProvider);
      final data = await client.getData('/rates', (d) => d as JsonMap);
      return ratesTable(data, exponents);
    }
    if (mode == AppMode.local) return await _public(ref, exponents);
  } catch (_) {
    // Rates only change how totals are shown; never fail a screen over them.
  }
  return null;
});

Future<RateTable?> _public(Ref ref, Map<String, int> exponents) async {
  final cache = ref.read(responseCacheProvider);
  final cached = await cache.read(_cacheKey);
  final table = cached is Map ? ratesTable(cached.cast(), exponents) : null;
  final age = table?.fetchedAt == null
      ? null
      : DateTime.now().toUtc().difference(table!.fetchedAt!);
  if (table != null && age != null && age < const Duration(hours: 12)) {
    return table;
  }
  try {
    final res = await ref
        .read(httpClientProvider)
        .get(Uri.parse(publicRatesUrl))
        .timeout(const Duration(seconds: 15));
    if (res.statusCode != 200) return table;
    final body = jsonDecode(res.body) as JsonMap;
    if (body['result'] != 'success') return table;
    final updated = (body['time_last_update_unix'] as num?)?.toInt();
    final data = <String, Object?>{
      'enabled': true,
      'base': body['base_code'],
      'source': body['provider'] ?? 'https://www.exchangerate-api.com',
      'publishedAt': updated == null
          ? null
          : DateTime.fromMillisecondsSinceEpoch(
              updated * 1000,
              isUtc: true,
            ).toIso8601String(),
      'fetchedAt': DateTime.now().toUtc().toIso8601String(),
      // Numbers keep their JSON text so no float ever reaches the math.
      'rates': {
        for (final e in (body['rates'] as Map).entries)
          e.key as String: _decimal(e.value),
      },
    };
    await cache.write(_cacheKey, data);
    return ratesTable(data, exponents) ?? table;
  } catch (_) {
    return table;
  }
}

/// JSON numbers decode as int or double. Dart prints a double with the
/// shortest text that round-trips, which is the provider's own decimal
/// text for the values these APIs send (for example 0.912345). Exponent
/// forms are expanded so the exact decimal parser accepts them.
String _decimal(Object? v) {
  if (v is int) return '$v';
  if (v is double) {
    final s = v.toString();
    if (!s.contains('e')) return s;
    return v.toStringAsFixed(20).replaceFirst(RegExp(r'\.?0+$'), '');
  }
  return '$v';
}

/// Parses the `GET /rates` shape. Null when rates are off or empty.
RateTable? ratesTable(JsonMap data, Map<String, int> exponents) {
  if (data['enabled'] == false) return null;
  final raw = (data['rates'] as Map?)?.cast<String, Object?>() ?? const {};
  final base = data['base'] as String?;
  if (base == null || raw.isEmpty) return null;
  return RateTable.parse(
    base: base,
    rates: {for (final e in raw.entries) e.key: '${e.value}'},
    exponents: exponents,
    publishedAt: DateTime.tryParse('${data['publishedAt']}'),
    fetchedAt: DateTime.tryParse('${data['fetchedAt']}'),
    source: (data['source'] as String?) ?? '',
  );
}

/// The currency every total is shown in: the user's chosen currency.
final displayCurrencyProvider = Provider<String>(
  (ref) => ref.watch(meProvider)?.defaultCurrency ?? 'USD',
);

/// Formatting in the display currency, used by every balance and total.
final moneyViewProvider = Provider<MoneyView>(
  (ref) => MoneyView(
    config: ref.watch(configValueProvider),
    rates: ref.watch(ratesProvider).value,
    currency: ref.watch(displayCurrencyProvider),
  ),
);

/// Shows amounts in one [currency] using [rates]. Amounts in currencies
/// without a rate are shown as they are, never dropped.
class MoneyView {
  const MoneyView({
    required this.config,
    required this.rates,
    required this.currency,
  });

  final AppConfig config;
  final RateTable? rates;
  final String currency;

  CurrencyInfo get info => config.currency(currency);

  /// [minor] of [from] in the display currency, or null without a rate.
  int? convert(int minor, String from) =>
      from == currency ? minor : rates?.convert(minor, from, currency);

  /// Formats an amount, converted when possible.
  String format(int minor, String from, {bool absolute = false}) {
    final v = convert(minor, from);
    return v == null
        ? Money.format(minor, config.currency(from), absolute: absolute)
        : Money.format(v, info, absolute: absolute);
  }

  /// The original amount, when it differs from what [format] shows.
  String? original(int minor, String from, {bool absolute = false}) =>
      from == currency || convert(minor, from) == null
      ? null
      : Money.format(minor, config.currency(from), absolute: absolute);

  /// Collapses per-currency balances into one display-currency balance plus
  /// those that could not be converted.
  List<Balance> combine(List<Balance> balances) {
    final nonZero = balances.where((b) => !b.isZero).toList();
    if (nonZero.isEmpty) return const [];
    final t = rates;
    if (t == null) {
      return nonZero.every((b) => b.currency == currency)
          ? [Balance(currency: currency, amount: _sum(nonZero))]
          : nonZero;
    }
    final total = t.total([
      for (final b in nonZero) (currency: b.currency, minor: b.amount),
    ], currency);
    final missing = total.missing.toSet();
    final converted = nonZero.any((b) => !missing.contains(b.currency));
    return [
      if (converted) Balance(currency: currency, amount: total.total),
      ...nonZero.where((b) => missing.contains(b.currency)),
    ];
  }

  /// Originals shown under a combined total, for example `€50.00 · ¥1,000`,
  /// or null when everything already is in the display currency.
  String? breakdown(List<Balance> balances) {
    final nonZero = balances.where((b) => !b.isZero).toList();
    if (nonZero.every((b) => b.currency == currency)) return null;
    if (rates == null) return null;
    return nonZero
        .where((b) => rates!.has(b.currency) || b.currency == currency)
        .map(
          (b) =>
              '${b.amount < 0 ? '−' : ''}${Money.format(b.amount, config.currency(b.currency), absolute: true)}',
        )
        .join(' · ');
  }

  static int _sum(List<Balance> list) =>
      list.fold(0, (sum, b) => sum + b.amount);
}
