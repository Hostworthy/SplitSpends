import 'package:uuid/uuid.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/optimistic.dart';

/// Connected mode: sends writes straight to the server, or queues them in
/// the outbox when it cannot be reached (and while older writes wait, to
/// keep their order). Reads come from the server (or the API client's
/// cached copies offline) with the queued writes laid over them, so a
/// change shows up as soon as it is made.
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
  }) async {
    // Read the queue first: a write sent meanwhile is then in both, and
    // shown once, rather than in neither.
    final pending = await _pending();
    PageResult<Expense> page;
    try {
      page = await _remote.list(query, limit: limit, offset: offset);
    } on ApiError catch (error) {
      // The server has nothing yet for a group still waiting to be created.
      if (!error.isNetwork || !pending.groups.contains(query.groupId)) rethrow;
      page = const PageResult(items: [], total: 0);
    }
    if (pending.isEmpty) return page;
    final items = <Expense>[];
    var total = page.total;
    for (final e in page.items) {
      final changed = pending.changed[e.id];
      if ((pending.deleted[e.id] ?? false) ||
          (changed != null && !_matches(changed, query))) {
        total--;
        continue;
      }
      items.add(changed ?? e);
    }
    if (offset == 0) {
      final listed = {for (final e in page.items) e.id};
      for (final e in pending.changed.values) {
        if (listed.contains(e.id) ||
            (pending.deleted[e.id] ?? false) ||
            !_matches(e, query)) {
          continue;
        }
        // Newest date first, as the server orders them.
        final at = items.indexWhere(
          (other) => other.date.compareTo(e.date) < 0,
        );
        items.insert(at < 0 ? items.length : at, e);
        total++;
      }
    }
    return PageResult(items: items, total: total);
  }

  @override
  Future<Expense> get(String id) async {
    final pending = await _pending();
    final deleted = pending.deleted[id];
    final expense = pending.changed[id] ?? await _remote.get(id);
    return deleted == null
        ? expense
        : expense.copyWith(deleted: deleted, pending: true);
  }

  /// What the queued writes will do, replayed in queue order.
  Future<_Pending> _pending() async {
    final pending = _Pending();
    final me = meId();
    final today = _today;
    for (final op in await _outbox.pending()) {
      final body = op.body;
      final json = body is Map ? body.cast<String, dynamic>() : null;
      try {
        switch ((op.method, op.path.split('/'))) {
          case ('POST', ['', 'expenses']) when json != null:
            final input = ExpenseInput.fromJson(json);
            final id = input.id;
            if (id == null) break;
            pending.changed[id] = optimisticExpense(
              input,
              id: id,
              meId: me,
              today: today,
            );
          case ('PUT', ['', 'expenses', final id]) when json != null:
            pending.changed[id] = optimisticExpense(
              ExpenseInput.fromJson(json),
              id: id,
              meId: me,
              today: today,
            );
          case ('DELETE', ['', 'expenses', final id]):
            pending.deleted[id] = true;
          case ('POST', ['', 'expenses', final id, 'restore']):
            pending.deleted[id] = false;
          case ('POST', ['', 'groups']) when json != null:
            if (json['id'] case final String id) pending.groups.add(id);
          case ('POST', ['', 'payments']) when json != null:
            final id = json['id'] as String?;
            if (id == null) break;
            pending.changed[id] = optimisticPayment(
              id: id,
              groupId: json['groupId'] as String?,
              fromUserId: json['fromUserId'] as String,
              toUserId: json['toUserId'] as String,
              amount: (json['amount'] as num).toInt(),
              currency: json['currency'] as String,
              date: json['date'] as String?,
              notes: json['notes'] as String?,
              meId: me,
              today: today,
            );
        }
      } on ApiError {
        // The ledger no longer accepts this queued write; the server will
        // answer it on replay, so it is not shown meanwhile.
      }
    }
    return pending;
  }

  /// The server's list filters, for expenses only this device knows yet.
  bool _matches(Expense e, ExpenseQuery query) {
    final q = query.q?.trim().toLowerCase() ?? '';
    final friend = query.friendId;
    return (query.groupId == null || e.groupId == query.groupId) &&
        (friend == null ||
            (e.shareOf(meId()) != null && e.shareOf(friend) != null)) &&
        (query.category == null || e.category == query.category) &&
        (q.isEmpty ||
            e.description.toLowerCase().contains(q) ||
            e.notes.toLowerCase().contains(q));
  }

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

  /// Queued like [delete] when writes wait, so it replays after a queued
  /// delete of the same expense instead of before it.
  @override
  Future<Expense> restore(String id) async {
    final known = await get(id);
    return _write(
      online: () => _remote.restore(id),
      offline: () => known.copyWith(deleted: false, pending: true),
      method: 'POST',
      path: '/expenses/$id/restore',
      label: 'Restore an expense',
    );
  }

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
      offline: () => optimisticPayment(
        id: paymentId,
        groupId: groupId,
        fromUserId: fromUserId,
        toUserId: toUserId,
        amount: amount,
        currency: currency,
        date: date,
        notes: notes,
        meId: meId(),
        today: _today,
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

/// Expenses as the queued writes leave them.
class _Pending {
  /// Created or edited, by id.
  final changed = <String, Expense>{};

  /// Deleted (true) or restored again (false), by id.
  final deleted = <String, bool>{};

  /// Groups still waiting to be created.
  final groups = <String>{};

  bool get isEmpty => changed.isEmpty && deleted.isEmpty;
}
