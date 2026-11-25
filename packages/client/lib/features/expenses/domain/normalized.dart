import 'package:splitspends_ledger/ledger.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

/// Validates [input] with the server's rules and computes every share. The
/// on-device ledger and the offline outbox both go through here, so they
/// agree with each other and with the server on what an input means.
NormalizedExpense normalized(
  ExpenseInput input, {
  required DateTime today,
  Set<String>? currencies,
  Set<String>? categories,
}) {
  try {
    return normalize(
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
      currencies: currencies,
      categories: categories,
    );
  } on LedgerError catch (error) {
    throw ApiError.validation(error.field, error.message);
  }
}

/// The fields of the API Expense that follow from [n]; callers add who
/// changed it and when.
JsonMap expenseJson(
  NormalizedExpense n, {
  required String id,
  required String? groupId,
  required String kind,
}) => {
  'id': id,
  'groupId': groupId,
  'kind': kind,
  'description': n.description,
  // Payments are filed under the payment category after the rules, which
  // keep it for payments only, as on the server.
  'category': kind == 'payment' ? paymentCategory : n.category,
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
};

/// What [meId] paid minus what they owe in an API Expense, as the server
/// reports it in `myBalance`.
int myBalance(JsonMap expense, String meId) {
  for (final s in (expense['shares'] as List).cast<JsonMap>()) {
    if (s['userId'] == meId) return (s['paid'] as int) - (s['owed'] as int);
  }
  return 0;
}
