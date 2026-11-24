import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

/// Filters for listing expenses.
class ExpenseQuery {
  const ExpenseQuery({this.groupId, this.friendId, this.q, this.category});

  final String? groupId;
  final String? friendId;
  final String? q;
  final String? category;

  @override
  bool operator ==(Object other) =>
      other is ExpenseQuery &&
      other.groupId == groupId &&
      other.friendId == friendId &&
      other.q == q &&
      other.category == category;

  @override
  int get hashCode => Object.hash(groupId, friendId, q, category);
}

/// Expenses, payments and comments.
abstract interface class ExpensesRepository {
  /// Newest first.
  Future<PageResult<Expense>> list(
    ExpenseQuery query, {
    int limit = 50,
    int offset = 0,
  });

  Future<Expense> get(String id);

  Future<Expense> create(ExpenseInput input);

  /// Full replace.
  Future<Expense> update(String id, ExpenseInput input);

  /// Soft delete; [restore] undoes it.
  Future<void> delete(String id);

  Future<Expense> restore(String id);

  Future<List<Comment>> comments(String id);

  Future<Comment> addComment(String id, String body);

  Future<void> deleteComment(String commentId);

  /// Records a payment (an expense with kind `payment`).
  Future<Expense> recordPayment({
    String? groupId,
    required String fromUserId,
    required String toUserId,
    required int amount,
    required String currency,
    String? date,
    String? notes,
  });
}
