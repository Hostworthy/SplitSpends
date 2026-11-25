import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/normalized.dart';

/// Builds the expense the server will create from [input], with the same
/// rules, so offline writes can show a correct result right away. The
/// result is marked pending until the server has it. Its version is
/// [version] when given, otherwise now.
Expense optimisticExpense(
  ExpenseInput input, {
  required String id,
  required String meId,
  required DateTime today,
  String kind = 'expense',
  String? version,
}) {
  final json = expenseJson(
    normalized(input, today: today),
    id: id,
    groupId: input.groupId,
    kind: kind,
  );
  final now = DateTime.now().toUtc().toIso8601String();
  return Expense.fromJson({
    ...json,
    'createdBy': meId,
    'updatedBy': meId,
    'createdAt': now,
    'updatedAt': version ?? now,
    'myBalance': myBalance(json, meId),
  }).copyWith(pending: true);
}

/// The payment the server will record: [fromUserId] paid [toUserId].
Expense optimisticPayment({
  required String id,
  String? groupId,
  required String fromUserId,
  required String toUserId,
  required int amount,
  required String currency,
  String? date,
  String? notes,
  required String meId,
  required DateTime today,
  String? version,
}) => optimisticExpense(
  ExpenseInput.payment(
    groupId: groupId,
    from: fromUserId,
    to: toUserId,
    amount: amount,
    currency: currency,
    date: date ?? '',
    notes: notes,
  ),
  id: id,
  meId: meId,
  today: today,
  kind: 'payment',
  version: version,
);
