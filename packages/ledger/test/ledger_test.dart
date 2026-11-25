import 'package:test/test.dart';

import 'package:splitspends_ledger/ledger.dart';

void main() {
  test('allocate keeps input order on ties and sums exactly', () {
    expect(even(100, 3), [34, 33, 33]);
    expect(even(-100, 3), [-34, -33, -33]);
    expect(
      allocate(1000, [
        Rational.integer(1),
        Rational.integer(2),
        Rational.integer(2),
      ]),
      [200, 400, 400],
    );
    final parts = allocate(98765, [
      Rational.tryParseDecimal('33.33')!,
      Rational.tryParseDecimal('33.33')!,
      Rational.tryParseDecimal('33.34')!,
    ]);
    expect(parts.reduce((a, b) => a + b), 98765);
    expect(() => allocate(1, [Rational.zero]), throwsA(isA<NoWeightsError>()));
  });

  test('decimal parsing is exact and strict', () {
    expect(
      Rational.tryParseDecimal('0.1')! + Rational.tryParseDecimal('0.2')!,
      Rational.tryParseDecimal('0.3'),
    );
    expect(Rational.tryParseDecimal('1/2'), isNull);
    expect(Rational.tryParseDecimal('1e3'), isNull);
    expect(Rational.tryParseDecimal(''), isNull);
    expect(Rational.tryParseDecimal('.5'), Rational(BigInt.one, BigInt.two));
  });

  test('normalize validates like the server', () {
    ExpenseRequest req({
      String desc = 'Taxi',
      int total = 100,
      String value = '1.23456',
    }) => ExpenseRequest(
      description: desc,
      currency: 'usd',
      total: total,
      method: 'shares',
      payers: const [PayerInput('a', 100)],
      splits: [SplitValueInput('a', value)],
    );
    expect(
      () => normalize(req(desc: '  '), today: DateTime.utc(2026)),
      throwsA(isA<LedgerError>().having((e) => e.field, 'f', 'description')),
    );
    expect(
      () => normalize(req(), today: DateTime.utc(2026)),
      throwsA(isA<LedgerError>().having((e) => e.field, 'f', 'splits')),
    );
    final n = normalize(req(value: '2'), today: DateTime.utc(2026, 3, 4));
    expect(n.currency, 'USD');
    expect(n.category, 'general');
    expect(formatDay(n.date), '2026-03-04');
    expect(n.shares.single.value, '2');
  });

  test('normalize caps people and items like the server', () {
    final many = [
      for (var i = 1; i <= maxPeople + 1; i++)
        '00000000-0000-0000-0000-${i.toRadixString(16).padLeft(12, '0')}',
    ];
    final a = many.first;
    final cases = {
      'payers': ExpenseRequest(
        description: 'Taxi',
        currency: 'USD',
        total: 100,
        payers: [
          PayerInput(a, 100),
          for (final id in many.skip(1)) PayerInput(id, 0),
        ],
        splits: [SplitValueInput(a)],
      ),
      'items': ExpenseRequest(
        description: 'Tea',
        currency: 'USD',
        total: maxItems + 1,
        method: 'itemized',
        payers: [PayerInput(a, maxItems + 1)],
        items: [
          for (var i = 0; i <= maxItems; i++) ItemInput('Tea', 1, [a]),
        ],
      ),
      'item users': ExpenseRequest(
        description: 'Cake',
        currency: 'USD',
        total: 100,
        method: 'itemized',
        payers: [PayerInput(a, 100)],
        items: [ItemInput('Cake', 100, many)],
      ),
    };
    cases.forEach((name, request) {
      expect(
        () => normalize(request, today: DateTime.utc(2026)),
        throwsA(
          isA<LedgerError>().having(
            (e) => e.field,
            'field',
            name == 'payers' ? 'payers' : 'items',
          ),
        ),
        reason: name,
      );
    });
  });

  test('recurrence overflows months like Go AddDate', () {
    expect(
      formatDay(nextOccurrence('monthly', DateTime.utc(2026, 1, 31))!),
      '2026-03-03',
    );
    expect(
      formatDay(nextOccurrence('yearly', DateTime.utc(2028, 2, 29))!),
      '2029-03-01',
    );
    expect(nextOccurrence('none', DateTime.utc(2026)), isNull);
  });

  test('a series behind schedule books only the latest occurrences', () {
    String day(DateTime d) => formatDay(d);
    var (dates, next) = missed(
      'monthly',
      DateTime.utc(2026, 2),
      DateTime.utc(2026, 3, 15),
    );
    expect(dates.map(day), ['2026-02-01', '2026-03-01']);
    expect(day(next), '2026-04-01');
    (dates, next) = missed(
      'weekly',
      DateTime.utc(2024, 3, 17),
      DateTime.utc(2026, 3, 15),
    );
    expect(dates, hasLength(catchUp));
    expect(day(dates.first), '2025-12-28');
    expect(day(dates.last), '2026-03-15');
    expect(day(next), '2026-03-22');
    (dates, next) = missed('weekly', DateTime.utc(1), DateTime.utc(2026));
    expect(dates, hasLength(catchUp));
    expect(next.isAfter(DateTime.utc(2026)), isTrue);
    (dates, _) = missed('none', DateTime.utc(2026), DateTime.utc(2026, 3));
    expect(dates, isEmpty);
  });

  test('sheet honours simplification per group plus non-group pairwise', () {
    const g = LedgerGroup('g', 'Trip', simplified: true);
    final sheet = sheetFor(
      'a',
      groups: [g],
      groupEntries: {
        'g': const [
          LedgerEntry(
            groupId: 'g',
            currency: 'EUR',
            shares: [
              LedgerShare('a', 300, 100),
              LedgerShare('b', 0, 100),
              LedgerShare('c', 0, 100),
            ],
          ),
        ],
      },
      nonGroup: const [
        LedgerEntry(
          currency: 'EUR',
          shares: [LedgerShare('a', 0, 50), LedgerShare('b', 100, 50)],
        ),
      ],
    );
    expect(sheet.friends['b']!.total['EUR'], 50);
    expect(sheet.friends['c']!.total['EUR'], 100);
    final t = sheet.totals();
    expect(t.net['EUR'], 150);
    expect(t.owed['EUR'], 150);
    expect(sheet.groups['g']!['EUR'], 200);
  });
}
