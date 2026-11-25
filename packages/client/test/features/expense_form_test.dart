import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/features/expenses/domain/expense.dart';
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
          ),
        ),
      );
    expect(form.currency, 'EUR');
    expect(form.method, SplitMethod.shares);
    expect(form.preview.owed['me'], 600);
    expect(form.canSave, isTrue);
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
}
