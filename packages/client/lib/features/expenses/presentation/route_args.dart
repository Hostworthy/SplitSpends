import 'package:splitspends/features/expenses/domain/expense.dart';

/// Arguments for the expense editor (the route's `extra`).
class ExpenseEditorArgs {
  const ExpenseEditorArgs({
    this.groupId,
    this.friendId,
    this.existing,
    this.draft,
  });

  final String? groupId;
  final String? friendId;

  /// Editing an existing expense.
  final Expense? existing;

  /// Prefill from AI (receipt scan or text draft).
  final ExpenseInput? draft;
}

/// Arguments for the settle-up screen.
class SettleArgs {
  const SettleArgs({
    this.groupId,
    this.fromUserId,
    this.toUserId,
    this.amount,
    this.currency,
    this.friendId,
  });

  final String? groupId;
  final String? fromUserId;
  final String? toUserId;
  final int? amount;
  final String? currency;
  final String? friendId;
}
