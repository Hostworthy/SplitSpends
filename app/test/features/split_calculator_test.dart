import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/split_calculator.dart';

void main() {
  group('allocate', () {
    test('spreads remainders one unit at a time and sums exactly', () {
      expect(allocate(1000, [1, 1, 1]), [334, 333, 333]);
      expect(allocate(100, [1, 2]), [33, 67]);
      expect(allocate(-1000, [1, 1, 1]), [-334, -333, -333]);
      final parts = allocate(98765, [3, 7, 11, 13]);
      expect(parts.reduce((a, b) => a + b), 98765);
    });

    test('zero weights get nothing', () {
      expect(allocate(100, [0, 1]), [0, 100]);
      expect(allocate(100, [0, 0]), [0, 0]);
    });
  });

  const people = ['a', 'b', 'c'];

  test('equal split', () {
    final p = previewSplit(
      method: SplitMethod.equal,
      total: 9000,
      participants: people,
    );
    expect(p.isValid, isTrue);
    expect(p.owed, {'a': 3000, 'b': 3000, 'c': 3000});
  });

  test('exact split must add up', () {
    var p = previewSplit(
      method: SplitMethod.exact,
      total: 1000,
      participants: people,
      values: {'a': 500, 'b': 300},
    );
    expect(p.isValid, isFalse);
    expect(p.remaining, 200);
    p = previewSplit(
      method: SplitMethod.exact,
      total: 1000,
      participants: people,
      values: {'a': 500, 'b': 300, 'c': 200},
    );
    expect(p.isValid, isTrue);
    expect(p.owed['c'], 200);
  });

  test('percent split must reach 100', () {
    var p = previewSplit(
      method: SplitMethod.percent,
      total: 1000,
      participants: people,
      values: {'a': 50, 'b': 25},
    );
    expect(p.isValid, isFalse);
    expect(p.remaining, 2500);
    p = previewSplit(
      method: SplitMethod.percent,
      total: 1000,
      participants: people,
      values: {'a': 33.34, 'b': 33.33, 'c': 33.33},
    );
    expect(p.isValid, isTrue);
    expect(p.owed.values.reduce((a, b) => a + b), 1000);
  });

  test('shares split is proportional', () {
    final p = previewSplit(
      method: SplitMethod.shares,
      total: 1200,
      participants: people,
      values: {'a': 2, 'b': 1, 'c': 1},
    );
    expect(p.owed, {'a': 600, 'b': 300, 'c': 300});
  });

  test('adjustment adds to an even split', () {
    final p = previewSplit(
      method: SplitMethod.adjustment,
      total: 1000,
      participants: people,
      values: {'a': 100},
    );
    expect(p.owed, {'a': 400, 'b': 300, 'c': 300});
    expect(
      previewSplit(
        method: SplitMethod.adjustment,
        total: 100,
        participants: people,
        values: {'a': 200},
      ).isValid,
      isFalse,
    );
  });

  test('itemized spreads tax and tip by subtotal', () {
    final p = previewSplit(
      method: SplitMethod.itemized,
      total: 3300,
      participants: const [],
      items: const [
        LineItem(name: 'Pizza', amount: 2000, userIds: ['a']),
        LineItem(name: 'Salad', amount: 1000, userIds: ['b']),
      ],
      tax: 300,
    );
    expect(p.isValid, isTrue);
    expect(p.owed, {'a': 2200, 'b': 1100});
  });

  test('itemized requires assignments and a matching total', () {
    expect(
      previewSplit(
        method: SplitMethod.itemized,
        total: 1000,
        participants: const [],
        items: const [LineItem(name: 'X', amount: 1000)],
      ).isValid,
      isFalse,
    );
    expect(
      previewSplit(
        method: SplitMethod.itemized,
        total: 999,
        participants: const [],
        items: const [
          LineItem(name: 'X', amount: 1000, userIds: ['a']),
        ],
      ).error,
      isNotNull,
    );
  });

  test('payers must add up to the total', () {
    expect(
      validatePayers(100, const [Payer(userId: 'a', amount: 100)]),
      isNull,
    );
    expect(
      validatePayers(100, const [Payer(userId: 'a', amount: 60)]),
      isNotNull,
    );
    expect(validatePayers(100, const []), isNotNull);
  });
}
