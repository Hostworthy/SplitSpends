import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/expenses/presentation/editor/itemized_section.dart';
import 'package:splitspends/features/expenses/presentation/editor/payers_section.dart';
import 'package:splitspends/features/expenses/presentation/editor/split_section.dart';

const _people = {
  'me': User(id: 'me', name: 'Ada Lovelace'),
  'ben': User(id: 'ben', name: 'Ben Okafor'),
};

ExpenseForm _form() =>
    ExpenseForm(meId: 'me', currency: 'USD', people: ['me', 'ben'])
      ..description = 'Dinner'
      ..total = 3000;

Future<void> _pump(
  WidgetTester tester,
  ExpenseForm form,
  Widget Function(ExpenseForm form) section,
) async {
  tester.view.physicalSize = const Size(420, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [configValueProvider.overrideWithValue(localConfig)],
      child: MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: ListenableBuilder(
              listenable: form,
              builder: (_, _) => section(form),
            ),
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

/// The labels screen readers announce for the text fields on screen.
List<String> _labels(WidgetTester tester) => [
  for (final element in find.byType(EditableText).evaluate())
    tester.getSemantics(find.byWidget(element.widget)).label,
];

void main() {
  for (final (method, labels) in [
    (SplitMethod.exact, ['Amount for you', 'Amount for Ben Okafor']),
    (
      SplitMethod.adjustment,
      ['Adjustment for you', 'Adjustment for Ben Okafor'],
    ),
    (SplitMethod.percent, ['Percent for you', 'Percent for Ben Okafor']),
    (SplitMethod.shares, ['Shares for you', 'Shares for Ben Okafor']),
  ]) {
    testWidgets('${method.name} fields say whose they are', (tester) async {
      final semantics = tester.ensureSemantics();
      final form = _form()..setMethod(method);
      await _pump(
        tester,
        form,
        (form) => SplitSection(form: form, people: _people),
      );
      final found = _labels(tester);
      expect(found, hasLength(2));
      for (final (i, label) in labels.indexed) {
        expect(found[i], contains(label));
      }
      semantics.dispose();
    });
  }

  testWidgets('payer fields say who paid', (tester) async {
    final semantics = tester.ensureSemantics();
    await _pump(
      tester,
      _form(),
      (form) => PayersSection(form: form, people: _people),
    );
    await tester.tap(find.byKey(const Key('payers-chip')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Several people'));
    await tester.pumpAndSettle();

    final found = _labels(tester);
    expect(found, hasLength(2));
    expect(found[0], contains('Paid by you'));
    expect(found[1], contains('Paid by Ben Okafor'));
    semantics.dispose();
  });

  testWidgets('item price fields name the item', (tester) async {
    final semantics = tester.ensureSemantics();
    final form = _form()..setMethod(SplitMethod.itemized);
    await _pump(
      tester,
      form,
      (form) => ItemizedSection(form: form, people: _people),
    );
    // The item name, its price, then tax and tip.
    expect(_labels(tester)[1], contains('Price of the item'));

    await tester.enterText(find.byType(TextField).first, 'Pasta');
    await tester.pump();
    expect(_labels(tester)[1], contains('Price of Pasta'));
    semantics.dispose();
  });
}
