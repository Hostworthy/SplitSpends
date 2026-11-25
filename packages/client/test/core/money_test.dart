import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/money/money.dart';

void main() {
  const usd = CurrencyInfo(
    code: 'USD',
    name: 'US Dollar',
    symbol: r'$',
    exponent: 2,
  );
  const jpy = CurrencyInfo(code: 'JPY', name: 'Yen', symbol: '¥', exponent: 0);
  const kwd = CurrencyInfo(
    code: 'KWD',
    name: 'Dinar',
    symbol: 'KD',
    exponent: 3,
  );

  group('Money.format', () {
    test('formats minor units with the currency exponent', () {
      expect(Money.format(1250, usd), r'$12.50');
      expect(Money.format(1250, jpy), '¥1,250');
      expect(Money.format(1250, kwd), 'KD 1.250');
    });

    test('absolute drops the sign, compact drops zero decimals', () {
      expect(Money.format(-999, usd, absolute: true), r'$9.99');
      expect(Money.format(5000, usd, compact: true), r'$50');
      expect(Money.format(5050, usd, compact: true), r'$50.50');
    });
  });

  group('Money.format exactness', () {
    test('keeps every digit of very large amounts', () {
      // 9,007,199,254,740,993 minor units cannot be represented as a double.
      expect(Money.format(9007199254740993, usd), r'$90,071,992,547,409.93');
    });

    test('negative amounts and letter symbols', () {
      expect(Money.format(-1205, usd), r'-$12.05');
      const aed = CurrencyInfo(
        code: 'AED',
        name: 'UAE Dirham',
        symbol: 'AED',
        exponent: 2,
      );
      expect(Money.format(150000, aed), 'AED 1,500.00');
    });
  });

  group('Money.toPlain', () {
    test('renders editable decimal text', () {
      expect(Money.toPlain(1250, 2), '12.50');
      expect(Money.toPlain(5, 2), '0.05');
      expect(Money.toPlain(-1205, 2), '-12.05');
      expect(Money.toPlain(1250, 0), '1250');
    });
  });

  group('Money.parse', () {
    test('parses common inputs without floating point error', () {
      expect(Money.parse('12.5', 2), 1250);
      expect(Money.parse('12,50', 2), 1250);
      expect(Money.parse('0.1', 2), 10);
      expect(Money.parse('1,234.56', 2), 123456);
      expect(Money.parse('1.234,56', 2), 123456);
      expect(Money.parse('1,234', 2), 123400);
      expect(Money.parse(r'$ 7', 2), 700);
      expect(Money.parse('-3.10', 2), -310);
      expect(Money.parse('1250', 0), 1250);
      expect(Money.parse('1,234,567', 2), 123456700);
      expect(Money.parse('1.234', 0), 1234);
    });

    test('reads a lone dot as the decimal point', () {
      expect(Money.parse('0.500', 2), 50);
      expect(Money.parse('12.500', 2), 1250);
      expect(Money.parse('9.999', 2), isNull);
      expect(Money.parse('1.234', 2), isNull);
      expect(Money.parse('1.234', 3), 1234);
    });

    test('rejects amounts too large to hold exactly', () {
      expect(Money.parse('184467440737095517', 2), isNull);
      expect(Money.parse('99999999999999', 2), isNull);
      expect(Money.parse('9999999999999', 2), 999999999999900);
      expect(Money.parse('999999999999999', 0), 999999999999999);
      expect(Money.parse('0000000000000001.00', 2), 100);
    });

    test('rejects invalid or over-precise values', () {
      expect(Money.parse('', 2), isNull);
      expect(Money.parse('abc', 2), isNull);
      expect(Money.parse('0.125', 2), isNull);
      expect(Money.parse('1.5', 0), isNull);
    });

    test('round trips with toPlain', () {
      for (final v in [0, 1, 99, 100, 123456, 7000001]) {
        expect(Money.parse(Money.toPlain(v, 2), 2), v);
        expect(Money.parse(Money.toPlain(v, 3), 3), v);
      }
    });
  });
}
