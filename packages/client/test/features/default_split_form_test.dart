import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/default_split_form.dart';

void main() {
  group('DefaultSplitForm', () {
    test('rent 60/40 by percent', () {
      final form = DefaultSplitForm(members: ['me', 'ben']);
      expect(form.method, isNull);
      expect(form.canSave, isFalse, reason: 'nothing changed');

      form.setMethod(SplitMethod.percent);
      expect(form.values, {'me': '50', 'ben': '50'});
      expect(form.remaining, '100% assigned');

      form.setValue('me', '60');
      expect(form.problem, 'Percentages must add up to 100');
      expect(form.remaining, '10% over');
      expect(form.canSave, isFalse);

      form.setValue('ben', '40');
      expect(form.problem, isNull);
      expect(form.canSave, isTrue);
      final split = form.split!;
      expect(split.method, SplitMethod.percent);
      expect(split.values, {'me': '60', 'ben': '40'});
    });

    test('even percentages add up to exactly 100', () {
      final form = DefaultSplitForm(members: ['a', 'b', 'c'])
        ..setMethod(SplitMethod.percent);
      expect(form.values, {'a': '33.3334', 'b': '33.3333', 'c': '33.3333'});
      expect(form.problem, isNull);
    });

    test('blank counts as zero and leaves someone out', () {
      final form = DefaultSplitForm(members: ['a', 'b', 'c'])
        ..setMethod(SplitMethod.shares);
      form.setValue('c', '');
      expect(form.problem, isNull);
      expect(form.split!.values, {'a': '1', 'b': '1', 'c': '0'});
      form
        ..setValue('a', '0')
        ..setValue('b', '0');
      expect(form.problem, 'At least one share must be above zero');
    });

    test('loads the saved split and can clear it', () {
      final form = DefaultSplitForm(
        members: ['me', 'ben'],
        initial: const DefaultSplit(
          method: SplitMethod.percent,
          values: {'me': '60', 'ben': '40'},
        ),
      );
      expect(form.changed, isFalse);
      form.setValue('me', '60.0');
      expect(form.changed, isFalse, reason: 'same number');
      form.setMethod(null);
      expect(form.canSave, isTrue);
      expect(form.split, isNull);
    });

    test('equal needs no values', () {
      final form = DefaultSplitForm(members: ['me', 'ben'])
        ..setMethod(SplitMethod.equal);
      expect(form.problem, isNull);
      expect(form.split!.values, isEmpty);
    });
  });

  group('ExpenseForm.applyDefault', () {
    ExpenseForm make() => ExpenseForm(
      meId: 'me',
      currency: 'EUR',
      groupId: 'flat',
      people: ['me', 'ben'],
    );

    test('a new expense starts with the group default', () {
      final form = make()
        ..description = 'Rent'
        ..total = 100000
        ..applyDefault(SplitMethod.percent, {'me': '60', 'ben': '40'});
      expect(form.method, SplitMethod.percent);
      expect(form.preview.owed, {'me': 60000, 'ben': 40000});
      final input = form.toInput();
      expect(input.splitMethod, SplitMethod.percent);
      expect(input.splits.map((s) => (s.userId, s.value)), [
        ('me', '60'),
        ('ben', '40'),
      ]);
    });

    test('the user can still change the split', () {
      final form = make()
        ..total = 1000
        ..applyDefault(SplitMethod.shares, {'me': '2', 'ben': '1'});
      expect(form.preview.owed, {'me': 667, 'ben': 333});
      form.setMethod(SplitMethod.equal);
      expect(form.preview.owed, {'me': 500, 'ben': 500});
    });

    test('a zero share leaves someone out', () {
      final form = make()
        ..description = 'Gym'
        ..total = 3000
        ..applyDefault(SplitMethod.shares, {'me': '1', 'ben': '0'});
      expect(form.toInput().splits.map((s) => s.userId), ['me']);
    });
  });
}
