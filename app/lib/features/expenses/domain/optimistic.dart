import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/ledger/ledger.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

/// Builds the expense the server will create from [input], with the same
/// rules, so offline writes can show a correct result right away.
Expense optimisticExpense(
  ExpenseInput input, {
  required String id,
  required String meId,
  required DateTime today,
  String kind = 'expense',
}) {
  final NormalizedExpense n;
  try {
    n = normalize(
      ExpenseRequest(
        description: input.description,
        currency: input.currency,
        total: input.total,
        date: input.date,
        category: input.category ?? '',
        notes: input.notes,
        method: input.splitMethod.wire,
        payers: [for (final p in input.payers) PayerInput(p.userId, p.amount)],
        splits: [
          for (final s in input.splits) SplitValueInput(s.userId, s.value),
        ],
        items: [
          for (final i in input.items) ItemInput(i.name, i.amount, i.userIds),
        ],
        tax: input.tax,
        tip: input.tip,
        recurrence: input.recurrence.name,
      ),
      today: today,
    );
  } on LedgerError catch (error) {
    throw ApiError.validation(error.field, error.message);
  }
  final mine = n.shares.where((s) => s.userId == meId).firstOrNull;
  final now = DateTime.now().toUtc().toIso8601String();
  return Expense.fromJson({
    'id': id,
    'groupId': input.groupId,
    'kind': kind,
    'description': n.description,
    'category': n.category,
    'currency': n.currency,
    'total': n.total,
    'date': formatDay(n.date),
    'notes': n.notes,
    'splitMethod': n.method,
    'shares': [
      for (final s in n.shares)
        {'userId': s.userId, 'paid': s.paid, 'owed': s.owed, 'value': s.value},
    ],
    'items': [
      for (final i in n.items)
        {'name': i.name, 'amount': i.amount, 'userIds': i.userIds},
    ],
    'tax': n.tax,
    'tip': n.tip,
    'recurrence': n.recurrence,
    'createdBy': meId,
    'updatedBy': meId,
    'createdAt': now,
    'updatedAt': now,
    'myBalance': mine == null ? 0 : mine.paid - mine.owed,
  });
}
