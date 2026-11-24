import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/rates/rates.dart';
import 'package:splitspends/features/dashboard/presentation/summary_card.dart';

final _exponents = {for (final c in localConfig.currencies) c.code: c.exponent};

final _table = ratesTable({
  'enabled': true,
  'base': 'USD',
  'source': 'https://www.exchangerate-api.com',
  'publishedAt': '2026-11-24T00:00:00Z',
  'fetchedAt': '2026-11-24T06:00:00Z',
  'rates': {'USD': '1', 'EUR': '0.912', 'JPY': '159.07', 'XAU': '0.0004'},
}, _exponents)!;

MoneyView _view(String currency, {bool rates = true}) => MoneyView(
  config: localConfig,
  rates: rates ? _table : null,
  currency: currency,
);

void main() {
  test('parses the rates response and ignores unknown currencies', () {
    expect(_table.base, 'USD');
    expect(_table.has('EUR'), isTrue);
    expect(_table.has('XAU'), isFalse);
    expect(_table.publishedAt, DateTime.utc(2026, 11, 24));
    expect(ratesTable({'enabled': false, 'rates': {}}, _exponents), isNull);
    expect(ratesTable({'enabled': true, 'rates': {}}, _exponents), isNull);
  });

  test('formats amounts in the display currency with the original', () {
    final view = _view('USD');
    expect(view.format(5000, 'EUR'), '\$54.82');
    expect(view.original(5000, 'EUR'), '€50.00');
    expect(view.original(5000, 'USD'), isNull);
    expect(view.format(1000, 'JPY'), '\$6.29');
    // No rate: shown as entered, never dropped.
    expect(view.format(100, 'SEK'), 'kr 1.00');
    expect(view.original(100, 'SEK'), isNull);
  });

  test('combines balances into one total and keeps the rest', () {
    final view = _view('USD');
    final combined = view.combine(const [
      Balance(currency: 'EUR', amount: 5000),
      Balance(currency: 'JPY', amount: 1000),
      Balance(currency: 'USD', amount: -2000),
      Balance(currency: 'SEK', amount: 300),
    ]);
    expect(combined.map((b) => (b.currency, b.amount)), [
      ('USD', 4111),
      ('SEK', 300),
    ]);
    expect(
      view.breakdown(const [
        Balance(currency: 'EUR', amount: 5000),
        Balance(currency: 'USD', amount: -2000),
      ]),
      '€50.00 · −\$20.00',
    );
    expect(view.breakdown(const [Balance(currency: 'USD', amount: 1)]), isNull);
  });

  test('without rates, amounts stay in their own currencies', () {
    final view = _view('USD', rates: false);
    expect(view.format(5000, 'EUR'), '€50.00');
    final combined = view.combine(const [
      Balance(currency: 'EUR', amount: 5000),
      Balance(currency: 'USD', amount: -2000),
    ]);
    expect(combined.length, 2);
    expect(view.breakdown(combined), isNull);
  });

  testWidgets('summary card shows one converted total', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [moneyViewProvider.overrideWithValue(_view('EUR'))],
        child: MaterialApp(
          theme: AppTheme.light(),
          home: const Scaffold(
            body: SummaryCard(
              summary: BalanceSummary(
                balances: [
                  Balance(currency: 'USD', amount: 10000),
                  Balance(currency: 'EUR', amount: -2000),
                ],
                owed: [Balance(currency: 'USD', amount: 10000)],
                owing: [Balance(currency: 'EUR', amount: -2000)],
              ),
            ),
          ),
        ),
      ),
    );
    // 100 USD is 91.20 EUR; minus 20 EUR owed.
    expect(find.text('+€71.20'), findsOneWidget);
    expect(find.text('€91.20'), findsOneWidget);
    expect(find.text('€20.00'), findsOneWidget);
    expect(find.textContaining('Shown in EUR'), findsOneWidget);
  });
}
