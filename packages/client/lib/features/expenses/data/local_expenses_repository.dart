import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends_ledger/ledger.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';

/// On-device expenses and payments, computed with the server's rules
/// (lib/core/ledger). Comments need a server.
class LocalExpensesRepository implements ExpensesRepository {
  LocalExpensesRepository(this._ledger);

  final LocalLedger _ledger;

  static const _needsServer = ApiError.needsServer(
    'Comments are shared through a server. Connect to one to use them.',
  );

  Expense _view(JsonMap e, String meId) {
    final mine = (e['shares'] as List)
        .cast<JsonMap>()
        .where((s) => s['userId'] == meId)
        .firstOrNull;
    return Expense.fromJson({
      ...e,
      'commentCount': 0,
      'myBalance': mine == null
          ? 0
          : (mine['paid'] as int) - (mine['owed'] as int),
    });
  }

  @override
  Future<PageResult<Expense>> list(
    ExpenseQuery query, {
    int limit = 50,
    int offset = 0,
  }) async {
    final db = await _ledger.db;
    final me = await _ledger.me(db);
    final memberOf = {
      for (final g in await _ledger.groups(db))
        if (LocalLedger.memberIds(g).contains(me.id)) g['id'] as String,
    };
    final q = query.q?.trim().toLowerCase();
    final matches =
        [
          for (final e in await _ledger.expenses(db))
            if (!LocalLedger.isDeleted(e) &&
                (memberOf.contains(e['groupId']) ||
                    LocalLedger.involves(e, me.id)) &&
                (query.groupId == null || e['groupId'] == query.groupId) &&
                (query.friendId == null ||
                    (LocalLedger.involves(e, me.id) &&
                        LocalLedger.involves(e, query.friendId!))) &&
                (query.category == null || e['category'] == query.category) &&
                (q == null ||
                    q.isEmpty ||
                    (e['description'] as String).toLowerCase().contains(q) ||
                    ((e['notes'] as String?) ?? '').toLowerCase().contains(q)))
              e,
        ]..sort((a, b) {
          final byDate = (b['date'] as String).compareTo(a['date'] as String);
          return byDate != 0
              ? byDate
              : (b['createdAt'] as String).compareTo(a['createdAt'] as String);
        });
    return PageResult(
      items: [
        for (final e in matches.skip(offset).take(limit)) _view(e, me.id),
      ],
      total: matches.length,
    );
  }

  Future<JsonMap> _raw(DatabaseClient c, String id) async {
    final e = await Stores.expenses.record(id).get(c);
    if (e == null) throw ApiError.notFound('Expense');
    return e;
  }

  @override
  Future<Expense> get(String id) async {
    final db = await _ledger.db;
    final me = await _ledger.me(db);
    return _view(await _raw(db, id), me.id);
  }

  /// Validates [input] and builds the stored record (API Expense shape).
  Future<JsonMap> _build(
    DatabaseClient c,
    ExpenseInput input, {
    required String id,
    String kind = 'expense',
    JsonMap? existing,
  }) async {
    final me = await _ledger.me(c);
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
          payers: [
            for (final p in input.payers) PayerInput(p.userId, p.amount),
          ],
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
        today: _ledger.today(),
        currencies: _ledger.currencies,
        categories: await _ledger.usableCategories(
          c,
          kept: existing?['category'] as String?,
        ),
      );
    } on LedgerError catch (error) {
      throw ApiError.validation(error.field, error.message);
    }
    final involved = {for (final s in n.shares) s.userId};
    if (!involved.contains(me.id)) {
      throw const ApiError.validation(
        'splits',
        'You need to be part of the expense.',
      );
    }
    if (input.groupId != null) {
      final g = await _ledger.group(input.groupId!, c);
      final members = LocalLedger.memberIds(g).toSet();
      if (!members.containsAll(involved)) {
        throw const ApiError.validation(
          'splits',
          'Everyone in the expense must be in the group.',
        );
      }
    } else {
      final known = (await _ledger.people(c)).keys.toSet()..add(me.id);
      if (!known.containsAll(involved)) {
        throw const ApiError.validation(
          'splits',
          'Add these people as friends first.',
        );
      }
    }
    final at = _ledger.stamp();
    final next = nextOccurrence(n.recurrence, n.date);
    return {
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
          {
            'userId': s.userId,
            'paid': s.paid,
            'owed': s.owed,
            'value': s.value,
          },
      ],
      // The original split input, so the expense can be re-sent as-is
      // (import to a server, editing).
      'payers': [
        for (final p in input.payers) {'userId': p.userId, 'amount': p.amount},
      ],
      'splits': [
        if (n.method != 'itemized')
          for (final s in input.splits) {'userId': s.userId, 'value': s.value},
      ],
      'items': [
        for (final i in n.items)
          {'name': i.name, 'amount': i.amount, 'userIds': i.userIds},
      ],
      'tax': n.tax,
      'tip': n.tip,
      'receiptId': null,
      'recurrence': n.recurrence,
      'nextDate': next == null ? null : formatDay(next),
      'createdBy': existing?['createdBy'] ?? me.id,
      'updatedBy': me.id,
      'createdAt': existing?['createdAt'] ?? at,
      'updatedAt': at,
      'deleted': false,
    };
  }

  Future<void> _log(
    DatabaseClient c,
    String kind,
    JsonMap e, [
    Map<String, Object?> extra = const {},
  ]) async {
    final groupId = e['groupId'] as String?;
    final group = groupId == null ? null : await _ledger.group(groupId, c);
    await _ledger.record(
      c,
      kind,
      groupId: groupId,
      expenseId: e['id'] as String,
      data: {
        'description': e['description'],
        'amount': e['total'],
        'currency': e['currency'],
        'groupName': ?group?['name'],
        ...extra,
      },
      involved: [
        for (final s in (e['shares'] as List).cast<JsonMap>())
          s['userId'] as String,
      ],
    );
  }

  @override
  Future<Expense> create(ExpenseInput input) async {
    final db = await _ledger.db;
    final id = input.id ?? _ledger.newId();
    await db.transaction((txn) async {
      if (await Stores.expenses.record(id).exists(txn)) return;
      final e = await _build(txn, input, id: id);
      await Stores.expenses.record(id).put(txn, e);
      await _log(txn, 'expense.created', e);
    });
    return get(id);
  }

  @override
  Future<Expense> update(String id, ExpenseInput input) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final old = await _raw(txn, id);
      if (old['kind'] == 'payment') {
        throw const ApiError.conflict('Payments cannot be edited.');
      }
      final e = await _build(txn, input, id: id, existing: old);
      await Stores.expenses.record(id).put(txn, {
        ...e,
        'groupId': old['groupId'],
      });
      await _log(txn, 'expense.updated', e);
    });
    return get(id);
  }

  Future<void> _setDeleted(String id, bool deleted) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final e = await _raw(txn, id);
      final me = await _ledger.me(txn);
      final next = {
        ...e,
        'deleted': deleted,
        'updatedBy': me.id,
        'updatedAt': _ledger.stamp(),
      };
      await Stores.expenses.record(id).put(txn, next);
      await _log(txn, deleted ? 'expense.deleted' : 'expense.restored', next);
    });
  }

  @override
  Future<void> delete(String id) => _setDeleted(id, true);

  @override
  Future<Expense> restore(String id) async {
    await _setDeleted(id, false);
    return get(id);
  }

  @override
  Future<List<Comment>> comments(String id) async => const [];

  @override
  Future<Comment> addComment(String id, String body) async =>
      throw _needsServer;

  @override
  Future<void> deleteComment(String commentId) async => throw _needsServer;

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
  }) async {
    if (fromUserId == toUserId) {
      throw const ApiError.validation(
        'toUserId',
        'Choose two different people.',
      );
    }
    final db = await _ledger.db;
    final paymentId = id ?? _ledger.newId();
    await db.transaction((txn) async {
      if (await Stores.expenses.record(paymentId).exists(txn)) return;
      final input = ExpenseInput(
        groupId: groupId,
        description: 'Payment',
        currency: currency,
        total: amount,
        date: date ?? formatDay(_ledger.today()),
        category: paymentCategory,
        notes: notes ?? '',
        splitMethod: SplitMethod.exact,
        payers: [Payer(userId: fromUserId, amount: amount)],
        splits: [SplitEntry(userId: toUserId, value: '$amount')],
      );
      final e = await _build(txn, input, id: paymentId, kind: 'payment');
      await Stores.expenses.record(paymentId).put(txn, e);
      final people = await _ledger.directory(txn);
      await _log(txn, 'payment.created', e, {
        'fromName': people[fromUserId]?['name'],
        'toName': people[toUserId]?['name'],
      });
    });
    return get(paymentId);
  }
}
