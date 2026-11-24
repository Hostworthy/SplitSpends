import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

/// HTTP implementation used in connected mode.
class RemoteExpensesRepository implements ExpensesRepository {
  RemoteExpensesRepository(this._api);

  final ApiClient _api;

  @override
  Future<PageResult<Expense>> list(
    ExpenseQuery query, {
    int limit = 50,
    int offset = 0,
  }) => _api.getList(
    '/expenses',
    Expense.fromJson,
    query: {
      'groupId': query.groupId,
      'friendId': query.friendId,
      'q': query.q,
      'category': query.category,
      'limit': limit,
      'offset': offset,
    },
  );

  @override
  Future<Expense> get(String id) =>
      _api.getData('/expenses/$id', (d) => Expense.fromJson(d as JsonMap));

  @override
  Future<Expense> create(ExpenseInput input) => _api.postData(
    '/expenses',
    input.toJson(),
    (d) => Expense.fromJson(d as JsonMap),
  );

  @override
  Future<Expense> update(String id, ExpenseInput input) => _api.putData(
    '/expenses/$id',
    input.toJson(),
    (d) => Expense.fromJson(d as JsonMap),
  );

  @override
  Future<void> delete(String id) => _api.delete('/expenses/$id');

  @override
  Future<Expense> restore(String id) => _api.postData(
    '/expenses/$id/restore',
    null,
    (d) => Expense.fromJson(d as JsonMap),
  );

  @override
  Future<List<Comment>> comments(String id) async => (await _api.getList(
    '/expenses/$id/comments',
    Comment.fromJson,
    query: {'limit': 200},
  )).items;

  @override
  Future<Comment> addComment(String id, String body) => _api.postData(
    '/expenses/$id/comments',
    {'body': body},
    (d) => Comment.fromJson(d as JsonMap),
  );

  @override
  Future<void> deleteComment(String commentId) =>
      _api.delete('/comments/$commentId');

  @override
  Future<Expense> recordPayment({
    String? groupId,
    required String fromUserId,
    required String toUserId,
    required int amount,
    required String currency,
    String? date,
    String? notes,
  }) => _api.postData('/payments', {
    'groupId': groupId,
    'fromUserId': fromUserId,
    'toUserId': toUserId,
    'amount': amount,
    'currency': currency,
    'date': ?date,
    'notes': ?notes,
  }, (d) => Expense.fromJson(d as JsonMap));
}
