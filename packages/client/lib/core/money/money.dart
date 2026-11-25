import 'package:intl/intl.dart';

import 'package:splitspends/core/config/app_config.dart';

/// Money helpers. Amounts are always integers in minor units; floats are
/// never used for arithmetic.
abstract final class Money {
  static final _grouping = NumberFormat('#,##0', 'en_US');

  /// Formats [minor] units of [currency], e.g. `1250, USD` → `$12.50`.
  /// Integer arithmetic only, always English digit grouping; [compact] drops
  /// a zero fraction (`$12` instead of `$12.00`).
  static String format(
    int minor,
    CurrencyInfo currency, {
    bool absolute = false,
    bool compact = false,
  }) {
    final value = absolute ? minor.abs() : minor;
    final unit = pow10(currency.exponent);
    final abs = value.abs();
    final whole = _grouping.format(abs ~/ unit);
    final frac = abs % unit;
    final showFrac = currency.exponent > 0 && !(compact && frac == 0);
    final number = showFrac
        ? '$whole.${frac.toString().padLeft(currency.exponent, '0')}'
        : whole;
    // Letter symbols such as "AED" or "CHF" read better with a space.
    final symbol = RegExp(r'^[A-Za-z]{2,}$').hasMatch(currency.symbol)
        ? '${currency.symbol} '
        : currency.symbol;
    return '${value < 0 ? '-' : ''}$symbol$number';
  }

  /// Plain decimal text for editing: `1250, exponent 2` → `12.50`.
  static String toPlain(int minor, int exponent) {
    final negative = minor < 0;
    final abs = minor.abs();
    if (exponent == 0) return '${negative ? '-' : ''}$abs';
    final unit = pow10(exponent);
    final whole = abs ~/ unit;
    final frac = (abs % unit).toString().padLeft(exponent, '0');
    return '${negative ? '-' : ''}$whole.$frac';
  }

  /// Parses user input like `12.5`, `12,50`, `1,234.56` or `-3` into minor
  /// units. Returns null when the text is not a valid amount, has more
  /// decimals than the currency allows, or is too large to hold exactly.
  ///
  /// A lone `.` is always the decimal point when the currency has decimals,
  /// so `9.999` in a 2-decimal currency is rejected rather than read as
  /// 9,999. A lone `,` followed by three digits reads as grouping (`1,234`).
  static int? parse(String input, int exponent) {
    var text = input.trim().replaceAll(RegExp(r'[\s_]'), '');
    if (text.isEmpty) return null;
    var negative = false;
    if (text.startsWith('-')) {
      negative = true;
      text = text.substring(1);
    } else if (text.startsWith('+')) {
      text = text.substring(1);
    }
    text = text.replaceAll(RegExp(r'[^0-9.,]'), '');
    if (text.isEmpty) return null;

    // Decide which separator is the decimal one: the last of `.`/`,` unless
    // it repeats or is a lone grouping comma (or a lone dot in a currency
    // without decimals).
    final lastDot = text.lastIndexOf('.');
    final lastComma = text.lastIndexOf(',');
    final sepIndex = lastDot > lastComma ? lastDot : lastComma;
    String whole = text;
    String frac = '';
    if (sepIndex >= 0) {
      final tail = text.substring(sepIndex + 1);
      final sep = text[sepIndex];
      final count = sep.allMatches(text).length;
      final head = text.substring(0, text.indexOf(sep));
      final looksLikeThousands =
          count > 1 ||
          (tail.length == 3 &&
              exponent < 3 &&
              (sep == ',' || exponent == 0) &&
              RegExp(r'^[1-9]\d{0,2}$').hasMatch(head));
      final otherSepPresent = sep == '.' ? lastComma >= 0 : lastDot >= 0;
      if (!looksLikeThousands || otherSepPresent) {
        whole = text.substring(0, sepIndex);
        frac = tail;
      }
    }
    whole = whole.replaceAll(RegExp(r'[.,]'), '').replaceFirst(
      RegExp(r'^0+'),
      '',
    );
    if (whole.isEmpty) whole = '0';
    // Fifteen digits in minor units stay exact both as a 64-bit int and as
    // a JavaScript number on the web, and cover the largest allowed total.
    if (whole.length + exponent > 15) return null;
    if (frac.contains(RegExp(r'[^0-9]'))) return null;
    if (frac.length > exponent) {
      // Allow trailing zeros beyond the exponent ("12.500" for 2 decimals).
      final extra = frac.substring(exponent);
      if (extra.replaceAll('0', '').isNotEmpty) return null;
      frac = frac.substring(0, exponent);
    }
    final wholeValue = int.tryParse(whole);
    if (wholeValue == null) return null;
    final fracValue = frac.isEmpty
        ? 0
        : int.parse(frac.padRight(exponent, '0'));
    final value = wholeValue * pow10(exponent) + fracValue;
    return negative ? -value : value;
  }

  static int pow10(int exponent) {
    var result = 1;
    for (var i = 0; i < exponent; i++) {
      result *= 10;
    }
    return result;
  }
}
