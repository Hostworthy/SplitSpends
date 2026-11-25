import 'dart:math';

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

  test('split values parse like money.Decimal in Go', () {
    expect(splitDecimal('+5'), Rational.integer(5));
    expect(splitDecimal('1.'), Rational.integer(1));
    expect(splitDecimal('9' * decimalLength), isNotNull);
    for (final bad in [
      '',
      ' 5',
      '0x10',
      '1_000',
      '0x1p4',
      '1e3',
      '1/2',
      '1.23456',
      '\ufeff5',
      '9' * (decimalLength + 1),
    ]) {
      expect(splitDecimal(bad), isNull, reason: bad);
    }
    expect(trimSpace('\u0085 5\u3000'), '5');
    expect(trimSpace('\ufeff5'), '\ufeff5');
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

  test('recurrence keeps the series day at month ends', () {
    String step(String r, DateTime d, [int? day]) =>
        formatDay(nextOccurrence(r, d, day)!);
    expect(step('monthly', DateTime.utc(2027, 1, 31)), '2027-02-28');
    expect(step('monthly', DateTime.utc(2027, 2, 28), 31), '2027-03-31');
    expect(step('monthly', DateTime.utc(2027, 3, 31), 31), '2027-04-30');
    expect(step('yearly', DateTime.utc(2028, 2, 29)), '2029-02-28');
    expect(step('yearly', DateTime.utc(2031, 2, 28), 29), '2032-02-29');
    expect(nextOccurrence('none', DateTime.utc(2026)), isNull);
    final (dates, next) = missed(
      'monthly',
      DateTime.utc(2026, 1, 31),
      DateTime.utc(2026, 4, 30),
    );
    expect(dates.map(formatDay), [
      '2026-01-31',
      '2026-02-28',
      '2026-03-31',
      '2026-04-30',
    ]);
    expect(formatDay(next), '2026-05-31');
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

  group('pairwise', () {
    // Balanced entries with several payers each, so most leave rounding
    // remainders between creditors.
    List<LedgerEntry> randomLedger(Random r) {
      final users = ['a', 'b', 'c', 'd', 'e', 'f'];
      List<int> cuts(int total, int n) {
        final parts = List.filled(n, 0);
        for (var k = 0; k < total; k++) {
          parts[r.nextInt(n)]++;
        }
        return parts;
      }

      return [
        for (var k = 0; k < 200; k++)
          () {
            users.shuffle(r);
            final people = users.sublist(0, 2 + r.nextInt(users.length - 1));
            final total = 1 + r.nextInt(500);
            final paid = cuts(total, 1 + r.nextInt(people.length));
            final owed = cuts(total, people.length);
            return LedgerEntry(
              currency: r.nextBool() ? 'USD' : 'EUR',
              shares: [
                for (var i = 0; i < people.length; i++)
                  LedgerShare(
                    people[i],
                    i < paid.length ? paid[i] : 0,
                    owed[i],
                  ),
              ],
            );
          }(),
      ];
    }

    test('debts add up to every member net', () {
      final r = Random(1);
      for (var round = 0; round < 50; round++) {
        final entries = randomLedger(r);
        final want = nets(entries);
        final got = <String, Balances>{};
        for (final d in pairwise(entries)) {
          got.putIfAbsent(d.to, () => {}).addAmount(d.currency, d.amount);
          got.putIfAbsent(d.from, () => {}).addAmount(d.currency, -d.amount);
        }
        want.forEach((id, b) {
          for (final cur in ['USD', 'EUR']) {
            expect(got[id]?[cur] ?? 0, b[cur] ?? 0, reason: '$round $id $cur');
          }
        });
      }
    });

    test('every creditor is owed what they fronted', () {
      const entry = LedgerEntry(
        currency: 'USD',
        shares: [
          LedgerShare('a', 2, 1),
          LedgerShare('b', 2, 1),
          LedgerShare('c', 0, 1),
          LedgerShare('d', 0, 1),
        ],
      );
      expect(pairwise([entry]), const [
        LedgerDebt('c', 'a', 'USD', 1),
        LedgerDebt('d', 'b', 'USD', 1),
      ]);
    });

    test('share order does not change the debts', () {
      final r = Random(2);
      final entries = randomLedger(r);
      final want = pairwise(entries);
      for (var k = 0; k < 20; k++) {
        final shuffled = [
          for (final e in entries)
            LedgerEntry(
              currency: e.currency,
              shares: [...e.shares]..shuffle(r),
            ),
        ];
        expect(pairwise(shuffled), want);
      }
    });
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
