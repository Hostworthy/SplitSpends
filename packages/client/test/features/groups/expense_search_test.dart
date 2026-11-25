import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/theme/theme.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/presentation/expense_feed_list.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/detail/expenses_tab.dart';

const _group = Group(id: 'g1', name: 'Flat');

Expense _expense(String id, String description) => Expense.fromJson({
  'id': id,
  'groupId': 'g1',
  'description': description,
  'currency': 'EUR',
  'total': 1200,
  'date': '2026-11-20',
  'shares': <Object>[],
});

/// Answers each search when the test says so; `x` fails.
class _Expenses extends Fake implements ExpensesRepository {
  final queries = <ExpenseQuery>[];
  final answers = <Completer<void>>[];

  @override
  Future<PageResult<Expense>> list(
    ExpenseQuery query, {
    int limit = 50,
    int offset = 0,
  }) async {
    queries.add(query);
    final answer = Completer<void>();
    answers.add(answer);
    await answer.future;
    if (query.q == 'x') throw const ApiError.network('No connection.');
    final items = [
      _expense('e1', 'Taxi to the airport'),
      _expense('e2', 'Groceries'),
    ].where((e) => query.q == null || e.description.contains(query.q!));
    return PageResult(items: [...items], total: items.length);
  }
}

Future<_Expenses> _pump(WidgetTester tester) async {
  final expenses = _Expenses();
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        meProvider.overrideWithValue(const Me(id: 'ada', name: 'Ada')),
        expensesRepositoryProvider.overrideWithValue(expenses),
      ],
      child: MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(body: GroupExpensesTab(group: _group)),
      ),
    ),
  );
  await tester.pump();
  return expenses;
}

/// Lets the latest list request finish.
Future<void> _answer(WidgetTester tester, _Expenses expenses) async {
  expenses.answers.last.complete();
  await tester.pumpAndSettle();
}

final _field = find.byKey(const Key('expense-search'));

void main() {
  testWidgets('a search stays in the box with a clear button', (tester) async {
    final expenses = await _pump(tester);
    expect(_field, findsOneWidget, reason: 'shown while the list loads');
    await _answer(tester, expenses);
    expect(find.text('Groceries'), findsOneWidget);

    await tester.enterText(_field, 'Taxi');
    await tester.testTextInput.receiveAction(TextInputAction.search);
    await tester.pump();
    expect(_field, findsOneWidget);
    await _answer(tester, expenses);

    expect(expenses.queries.last.q, 'Taxi');
    expect(expenses.queries.last.groupId, 'g1');
    expect(find.text('Groceries'), findsNothing);
    expect(
      find.descendant(of: _field, matching: find.text('Taxi')),
      findsOneWidget,
    );

    // The unfiltered list is still loaded, so clearing shows it at once.
    await tester.tap(find.byTooltip('Clear search'));
    await tester.pumpAndSettle();
    expect(find.text('Groceries'), findsOneWidget);
    expect(
      find.descendant(of: _field, matching: find.text('Taxi')),
      findsNothing,
    );
    expect(find.byTooltip('Clear search'), findsNothing);
  });

  testWidgets('a failed search keeps the box so it can be cleared', (
    tester,
  ) async {
    final expenses = await _pump(tester);
    await _answer(tester, expenses);

    await tester.enterText(_field, 'x');
    await tester.testTextInput.receiveAction(TextInputAction.search);
    await tester.pump();
    await _answer(tester, expenses);

    expect(find.text('You are offline'), findsOneWidget);
    expect(find.descendant(of: _field, matching: find.text('x')), findsOne);
    await tester.tap(find.byTooltip('Clear search'));
    await tester.pumpAndSettle();
    expect(find.text('Groceries'), findsOneWidget);
  });

  testWidgets('the list keeps its category filter while searching', (
    tester,
  ) async {
    final expenses = _Expenses();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          meProvider.overrideWithValue(const Me(id: 'ada', name: 'Ada')),
          expensesRepositoryProvider.overrideWithValue(expenses),
        ],
        child: MaterialApp(
          theme: AppTheme.light(),
          home: const Scaffold(
            body: ExpenseFeedList(
              query: ExpenseQuery(groupId: 'g1', category: 'food.dining'),
            ),
          ),
        ),
      ),
    );
    await tester.pump();
    await _answer(tester, expenses);
    await tester.enterText(_field, 'Taxi');
    await tester.testTextInput.receiveAction(TextInputAction.search);
    await tester.pump();
    await _answer(tester, expenses);
    expect(
      [for (final q in expenses.queries) q.category],
      ['food.dining', 'food.dining'],
    );
  });
}
