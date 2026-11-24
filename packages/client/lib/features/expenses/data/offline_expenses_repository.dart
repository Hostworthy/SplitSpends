import 'package:uuid/uuid.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/optimistic.dart';

/// Connected mode: sends writes straight to the server, or queues them in
/// the outbox when it cannot be reached (and while older writes wait, to
/// keep their order). Reads pass through (the API client serves cached
/// copies offline).
class OfflineExpensesRepository implements ExpensesRepository {
  OfflineExpensesRepository(
    this._remote,
    this._outbox, {
    required this.meId,
    required this.onQueued,
  });

  final ExpensesRepository _remote;
  final Outbox _outbox;
  final String Function() meId;

  /// Lets the app refresh its "waiting to sync" state.
  final void Function() onQueued;

  static const _uuid = Uuid();

  DateTime get _today {
    final n = DateTime.now();
    return DateTime.utc(n.year, n.month, n.day);
  }

  /// Runs [online] unless writes are already waiting; on a network failure
  /// (or when queueing) stores the request and returns [offline].
  Future<T> _write<T>({
    required Future<T> Function() online,
    required T Function() offline,
    required String method,
    required String path,
    Object? body,
    required String label,
  }) async {
    // Validate first so bad input never reaches the queue.
    final result = offline();
    if ((await _outbox.pending()).isEmpty) {
      try {
        return await online();
      } on ApiError catch (error) {
        if (!error.isNetwork) rethrow;
      }
    }
    await _outbox.add(method, path, body: body, label: label);
    onQueued();
    return result;
  }

  @override
  Future<PageResult<Expense>> list(
    ExpenseQuery query, {
    int limit = 50,
    int offset = 0,
  }) => _remote.list(query, limit: limit, offset: offset);

  @override
  Future<Expense> get(String id) => _remote.get(id);

  @override
  Future<Expense> create(ExpenseInput input) {
    final withId = input.withId(input.id ?? _uuid.v4());
    return _write(
      online: () => _remote.create(withId),
      offline: () => optimisticExpense(
        withId,
        id: withId.id!,
        meId: meId(),
        today: _today,
      ),
      method: 'POST',
      path: '/expenses',
      body: withId.toJson(),
      label: 'Add “${input.description}”',
    );
  }

  @override
  Future<Expense> update(String id, ExpenseInput input) => _write(
    online: () => _remote.update(id, input),
    offline: () =>
        optimisticExpense(input, id: id, meId: meId(), today: _today),
    method: 'PUT',
    path: '/expenses/$id',
    body: input.toJson(),
    label: 'Edit “${input.description}”',
  );

  @override
  Future<void> delete(String id) => _write<void>(
    online: () => _remote.delete(id),
    offline: () {},
    method: 'DELETE',
    path: '/expenses/$id',
    label: 'Delete an expense',
  );

  @override
  Future<Expense> restore(String id) => _remote.restore(id);

  @override
  Future<List<Comment>> comments(String id) => _remote.comments(id);

  @override
  Future<Comment> addComment(String id, String body) =>
      _remote.addComment(id, body);

  @override
  Future<void> deleteComment(String commentId) =>
      _remote.deleteComment(commentId);

  @override
  Future<Expense> recordPayment({
    String? id,
    String? groupId,
    required String fromUserId,
    required String toUserId,
    required int amount,
    required String currency,
    String? date,
    String? notes,
  }) {
    final paymentId = id ?? _uuid.v4();
    return _write(
      online: () => _remote.recordPayment(
        id: paymentId,
        groupId: groupId,
        fromUserId: fromUserId,
        toUserId: toUserId,
        amount: amount,
        currency: currency,
        date: date,
        notes: notes,
      ),
      offline: () => optimisticExpense(
        ExpenseInput(
          groupId: groupId,
          description: 'Payment',
          currency: currency,
          total: amount,
          date: date ?? '',
          category: 'payment',
          notes: notes ?? '',
          splitMethod: SplitMethod.exact,
          payers: [Payer(userId: fromUserId, amount: amount)],
          splits: [SplitEntry(userId: toUserId, value: '$amount')],
        ),
        id: paymentId,
        meId: meId(),
        today: _today,
        kind: 'payment',
      ),
      method: 'POST',
      path: '/payments',
      body: {
        'id': paymentId,
        'groupId': groupId,
        'fromUserId': fromUserId,
        'toUserId': toUserId,
        'amount': amount,
        'currency': currency,
        'date': ?date,
        'notes': ?notes,
      },
      label: 'Record a payment',
    );
  }
}
