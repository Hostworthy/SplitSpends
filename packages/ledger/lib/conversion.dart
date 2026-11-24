import 'package:splitspends_ledger/rational.dart';

/// A value in minor units of one currency.
typedef Amount = ({String currency, int minor});

/// Exchange rates for showing totals in one currency, mirroring
/// `packages/core/domains/rates`. Conversion never changes stored amounts or
/// balances; it only feeds display.
///
/// [rates] maps a currency code to units of that currency per one [base].
/// [exponents] gives each currency's minor-unit exponent (2 for USD, 0 for
/// JPY, 3 for BHD).
class RateTable {
  RateTable({
    required this.base,
    required Map<String, Rational> rates,
    required this.exponents,
    this.publishedAt,
    this.fetchedAt,
    this.source = '',
  }) : rates = {...rates, base: Rational.integer(1)};

  /// Builds a table from decimal strings such as `"0.912"`, dropping
  /// invalid or non-positive values like the server does.
  factory RateTable.parse({
    required String base,
    required Map<String, String> rates,
    required Map<String, int> exponents,
    DateTime? publishedAt,
    DateTime? fetchedAt,
    String source = '',
  }) {
    final parsed = <String, Rational>{};
    rates.forEach((code, text) {
      final r = Rational.tryParseDecimal(text);
      if (r != null && r.sign > 0 && exponents.containsKey(code)) {
        parsed[code] = r;
      }
    });
    return RateTable(
      base: base,
      rates: parsed,
      exponents: exponents,
      publishedAt: publishedAt,
      fetchedAt: fetchedAt,
      source: source,
    );
  }

  final String base;
  final Map<String, Rational> rates;
  final Map<String, int> exponents;
  final DateTime? publishedAt;
  final DateTime? fetchedAt;
  final String source;

  /// Whether [code] can be converted.
  bool has(String code) => rates.containsKey(code);

  /// [minor] units of [from] in minor units of [to], rounded half away from
  /// zero, or null when a rate or exponent is missing.
  int? convert(int minor, String from, String to) {
    final v = _exact(minor, from, to);
    return v == null ? null : _round(v);
  }

  /// Sum of [amounts] in [to], adding exact values and rounding once.
  /// [missing] lists currencies that could not be converted (left out).
  ({int total, List<String> missing}) total(
    Iterable<Amount> amounts,
    String to,
  ) {
    var sum = Rational.zero;
    final missing = <String>{};
    for (final a in amounts) {
      final v = _exact(a.minor, a.currency, to);
      if (v == null) {
        missing.add(a.currency);
      } else {
        sum = sum + v;
      }
    }
    return (total: _round(sum), missing: missing.toList()..sort());
  }

  Rational? _exact(int minor, String from, String to) {
    final value = Rational.integer(minor);
    final ef = exponents[from];
    final et = exponents[to];
    if (ef == null || et == null) return null;
    if (from == to) return value;
    final rf = rates[from];
    final rt = rates[to];
    if (rf == null || rt == null) return null;
    final scale = Rational(BigInt.from(10).pow((et - ef).abs()));
    final converted = value * rt / rf;
    return et >= ef ? converted * scale : converted / scale;
  }

  static int _round(Rational v) {
    final n = v.numerator.abs();
    final d = v.denominator;
    var q = n ~/ d;
    if ((n % d) * BigInt.two >= d) q += BigInt.one;
    return (v.sign < 0 ? -q : q).toInt();
  }
}
