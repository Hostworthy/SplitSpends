import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:splitspends_ledger/ledger.dart';

import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/split_calculator.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';

void main() {
  ExpenseForm make() => ExpenseForm(
    meId: 'me',
    currency: 'USD',
    groupId: 'g',
    people: ['me', 'a', 'b'],
  );

  test('defaults to me paying and an equal split', () {
    final form = make()
      ..description = 'Dinner'
      ..total = 3000;
    expect(form.canSave, isTrue);
    final input = form.toInput();
    expect(input.payers.single.userId, 'me');
    expect(input.payers.single.amount, 3000);
    expect(input.splits.map((s) => s.userId), ['me', 'a', 'b']);
  });

  test('excluding someone from an equal split', () {
    final form = make()
      ..description = 'Taxi'
      ..total = 1000;
    form.update(() => form.included.remove('b'));
    expect(form.preview.owed, {'me': 500, 'a': 500});
    expect(form.toInput().splits.length, 2);
  });

  test('multiple payers must add up', () {
    final form = make()
      ..description = 'Groceries'
      ..total = 1000;
    form.setMultiplePayers({'me': 600, 'a': 300});
    expect(form.canSave, isFalse);
    form.setMultiplePayers({'me': 600, 'a': 400});
    expect(form.canSave, isTrue);
  });

  test('loads an existing expense for editing', () {
    final form = make()
      ..load(
        ExpenseInput.fromExpense(
          const Expense(
            id: 'e',
            description: 'Rent',
            currency: 'EUR',
            total: 900,
            date: '2026-11-01',
            splitMethod: SplitMethod.shares,
            shares: [
              Share(userId: 'me', paid: 900, owed: 600, value: '2'),
              Share(userId: 'a', paid: 0, owed: 300, value: '1'),
            ],
            updatedAt: '2026-11-02T08:00:00Z',
          ),
        ),
      );
    expect(form.currency, 'EUR');
    expect(form.method, SplitMethod.shares);
    expect(form.preview.owed['me'], 600);
    expect(form.canSave, isTrue);
    expect(
      form.toInput().toJson()['updatedAt'],
      '2026-11-02T08:00:00Z',
      reason: 'the edit names the version it started from',
    );
  });

  test('an edited expense keeps its group', () {
    final form = ExpenseForm(
      meId: 'me',
      currency: 'USD',
      groupId: 'g',
      groupLocked: true,
      people: ['me', 'a', 'b'],
    )..description = 'Dinner';
    form.update(() => form.total = 3000);
    form.setPeople(['me', 'c'], group: 'h');
    expect(form.groupId, 'g');
    expect(form.toInput().groupId, 'g');
  });

  test('switching to itemized seeds an item for everyone', () {
    final form = make()
      ..description = 'Lunch'
      ..total = 1000;
    form.setMethod(SplitMethod.itemized);
    expect(form.items.single.userIds, {'me', 'a', 'b'});
    form.items.single.amount = 900;
    form.update(() => form.tip = 100);
    expect(form.canSave, isTrue);
  });

  test('switching from USD to JPY does not keep 1250 as the total', () {
    final form = make()
      ..description = 'Dinner'
      ..total = 1250;
    form.setMultiplePayers({'me': 1000, 'a': 250});
    form.setMethod(SplitMethod.exact);
    form.update(() => form.values.addAll({'me': '625', 'a': '625'}));
    final cleared = form.setCurrency('JPY', from: 2, to: 0);
    expect(cleared, isTrue);
    expect(form.currency, 'JPY');
    expect(form.total, isNull);
    expect(form.payers, {'me': 0, 'a': 0});
    expect(form.values, isEmpty);
    expect(form.canSave, isFalse);
  });

  test('switching to fewer decimals keeps whole amounts', () {
    final form = make()
      ..description = 'Dinner'
      ..total = 1200;
    expect(form.setCurrency('JPY', from: 2, to: 0), isFalse);
    expect(form.total, 12);
    expect(form.toInput().payers.single.amount, 12);
  });

  test('switching to more decimals rescales every amount', () {
    final form = make()
      ..description = 'Lunch'
      ..total = 1250;
    form.setMethod(SplitMethod.itemized);
    form.items.single.amount = 1000;
    form.update(() {
      form.tax = 150;
      form.tip = 100;
    });
    form.setMultiplePayers({'me': 1250});
    expect(form.setCurrency('KWD', from: 2, to: 3), isFalse);
    expect(form.total, 12500);
    expect(form.payers, {'me': 12500});
    expect(form.items.single.amount, 10000);
    expect(form.tax, 1500);
    expect(form.tip, 1000);
    expect(form.canSave, isTrue);
  });

  test('adjustments are rescaled but percentages are not', () {
    final form = make()..total = 1000;
    form.setMethod(SplitMethod.adjustment);
    form.update(() => form.values['a'] = '-150');
    form.setCurrency('KWD', from: 2, to: 3);
    expect(form.values, {'a': '-1500'});

    form.setMethod(SplitMethod.percent);
    form.update(() => form.values.addAll({'me': '50', 'a': '50'}));
    form.setCurrency('JPY', from: 3, to: 0);
    expect(form.values, {'me': '50', 'a': '50'});
  });

  test('switching between currencies with the same exponent keeps amounts', () {
    final form = make()
      ..description = 'Dinner'
      ..total = 1250;
    form.setMultiplePayers({'me': 1000, 'a': 250});
    expect(form.setCurrency('EUR', from: 2, to: 2), isFalse);
    expect(form.currency, 'EUR');
    expect(form.total, 1250);
    expect(form.payers, {'me': 1000, 'a': 250});
    expect(form.canSave, isTrue);
  });
  group('edit round-trip', () {
    final vectors =
        (jsonDecode(File('../../spec/vectors.json').readAsStringSync())
                as Map<String, dynamic>)['splits']
            as List;
    const outsider = 'not-in-the-expense';

    /// The expense the server stores for a split vector's input.
    Expense stored(JsonMap c, {required bool values}) {
      final i = c['input'] as JsonMap;
      final n = normalize(
        ExpenseRequest(
          description: 'Round trip',
          currency: (i['currency'] as String?) ?? 'USD',
          total: i['total'] as int,
          date: i['date'] as String?,
          method: (i['method'] as String?) ?? '',
          payers: [
            for (final p in (i['payers'] as List).cast<JsonMap>())
              PayerInput(p['userId'] as String, p['amount'] as int),
          ],
          splits: [
            for (final s in (i['splits'] as List? ?? const []).cast<JsonMap>())
              SplitValueInput(s['userId'] as String, s['value']?.toString()),
          ],
          items: [
            for (final it in (i['items'] as List? ?? const []).cast<JsonMap>())
              ItemInput(
                it['name'] as String,
                it['amount'] as int,
                (it['userIds'] as List).cast<String>(),
              ),
          ],
          tax: (i['tax'] as int?) ?? 0,
          tip: (i['tip'] as int?) ?? 0,
        ),
        today: c['today'] == null
            ? DateTime.utc(2026)
            : parseDay(c['today'] as String)!,
      );
      return Expense.fromJson({
        'id': 'e',
        'description': n.description,
        'currency': n.currency,
        'total': n.total,
        'date': formatDay(n.date),
        'splitMethod': n.method,
        'shares': [
          for (final s in n.shares)
            {
              'userId': s.userId,
              'paid': s.paid,
              'owed': s.owed,
              'value': values ? s.value : null,
            },
        ],
        'items': [
          for (final it in n.items)
            {'name': it.name, 'amount': it.amount, 'userIds': it.userIds},
        ],
        'tax': n.tax,
        'tip': n.tip,
      });
    }

    Map<String, int> owing(Map<String, int> owed) => {
      for (final e in owed.entries)
        if (e.value != 0) e.key: e.value,
    };

    for (final method in SplitMethod.values) {
      test('edit round-trip keeps owed amounts for ${method.wire}', () {
        final cases = [
          for (final c in vectors.cast<JsonMap>())
            if (c['owed'] != null &&
                ((c['input'] as JsonMap)['method'] ?? 'equal') == method.wire)
              c,
        ];
        expect(cases, isNotEmpty);
        for (final c in cases) {
          // The API may send a share without its value; an exact split then
          // starts from what each person owes.
          for (final values in [true, if (method == SplitMethod.exact) false]) {
            final expense = stored(c, values: values);
            final me = expense.payers.first.userId;
            final i = c['input'] as JsonMap;
            final form = ExpenseForm(
              meId: me,
              currency: 'USD',
              // Remainders go to people in the order they are listed, and the
              // API returns shares by user id, so the editor lists people in
              // the order the expense was entered, as a group's members are.
              // An outsider can take part but is not in the expense; the
              // editor splits an adjustment between everyone it shows, so
              // that round trip has none.
              people: <String>{
                for (final s in (i['splits'] as List? ?? const []))
                  (s as JsonMap)['userId'] as String,
                for (final it in (i['items'] as List? ?? const []))
                  ...((it as JsonMap)['userIds'] as List).cast<String>(),
                if (method != SplitMethod.adjustment) outsider,
              }.toList(),
            )..load(ExpenseInput.fromExpense(expense));
            final input = form.toInput();
            final preview = previewSplit(
              method: input.splitMethod,
              total: input.total,
              participants: [for (final s in input.splits) s.userId],
              values: {
                for (final s in input.splits)
                  if (s.value != null) s.userId: s.value!,
              },
              items: input.items,
              tax: input.tax,
              tip: input.tip,
            );
            final reason = '${c['name']}, values: $values';
            expect(preview.error, isNull, reason: reason);
            // A zero share is dropped on save; nobody's amount changes.
            expect(
              owing(preview.owed),
              owing({for (final s in expense.shares) s.userId: s.owed}),
              reason: reason,
            );
            expect(form.canSave, isTrue, reason: reason);
          }
        }
      });
    }
  });
}
