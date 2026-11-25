import 'dart:convert';

import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/storage/local_db.dart';

/// The account a queued write belongs to: one user on one server. [user] is
/// null while the account is not known yet (an offline start before this
/// device ever recorded it).
typedef Owner = ({String server, String? user});

/// A write waiting to reach the server.
class OutboxOp {
  const OutboxOp({
    required this.key,
    required this.method,
    required this.path,
    required this.label,
    this.body,
    this.attempts = 0,
    this.error,
    this.stuck = false,
  });

  final int key;
  final String method;
  final String path;
  final String label;
  final Object? body;

  /// Times the server failed on this write.
  final int attempts;

  /// The server's last answer, for people deciding what to do with it.
  final String? error;

  /// The server keeps failing on this write: it waits for Retry or Discard
  /// and no longer holds back the writes queued after it.
  final bool stuck;

  /// The id of the record this write creates or changes.
  String get target {
    final payload = body;
    if (payload is Map && payload['id'] is String) {
      return payload['id'] as String;
    }
    return path.split('/').last;
  }

  /// True when this write names one of [ids] (all client-made UUIDs), in its
  /// path or body: an edit of that record, or an expense in that group.
  bool touches(Set<String> ids) {
    if (ids.isEmpty) return false;
    final text = '$path ${jsonEncode(body)}';
    return ids.any(text.contains);
  }
}

/// Result of one replay pass.
class FlushResult {
  const FlushResult({
    this.sent = 0,
    this.rejected = const [],
    this.offline = false,
    this.retryAfter,
  });

  final int sent;

  /// Labels of changes the server refused (dropped: the server wins).
  final List<String> rejected;

  /// True when the server could not take writes now and replay stopped.
  final bool offline;

  /// How long the server asked to be left alone, when it said so.
  final Duration? retryAfter;
}

/// Persisted queue of writes made while offline. Creates carry client ids,
/// so replaying after a lost response never duplicates anything.
///
/// Ownership: every write is stamped with the [Owner] that made it, and an
/// Outbox only sees, counts and replays its own owner's writes. Writes of
/// another account or server stay on the device untouched until that
/// account signs in again, so nothing is ever sent with someone else's token
/// or to another server. [others] counts them for the sign-in screens, which
/// also offer to discard them.
///
/// Sign-out: a sign-out the user asks for discards the account's writes
/// after the account screen warned about them ([clear]). An involuntary one
/// (the server answered 401: the session expired or was revoked) keeps them,
/// and signing back in to the same account replays them.
///
/// Replay answers, per write in queue order:
/// - success: the write is done and removed.
/// - the API's own error envelope with a verdict on the request
///   (validation, conflict, not found, forbidden, approval): dropped and
///   reported, the server wins.
/// - a server failure inside the API (5xx with the envelope): counted;
///   after [stuckAfter] failures the write is marked stuck and replay moves
///   past it, holding back only later writes that name the same record
///   (so an edit never runs before its create, nor an expense before its
///   group).
/// - anything else stops the pass and keeps the write for later: no
///   connection, 401, 408, 429 (honouring Retry-After), 502 to 504, and
///   answers without the envelope, which come from a proxy rather than the
///   API.
class Outbox {
  Outbox(this._open, {required this.owner});

  final Future<Database> Function() _open;

  /// The signed-in account, null when nobody is signed in.
  final Owner? owner;

  /// Server failures on one write before it is marked stuck.
  static const stuckAfter = 3;

  static const _verdicts = {
    'VALIDATION_ERROR',
    'NOT_FOUND',
    'FORBIDDEN',
    'CONFLICT',
    'APPROVAL_PENDING',
    'APPROVAL_DECLINED',
  };

  Future<void> add(
    String method,
    String path, {
    Object? body,
    required String label,
  }) async {
    final account = owner;
    if (account == null) {
      throw StateError('Changes can only be queued while signed in.');
    }
    await Stores.outbox.add(await _open(), {
      'method': method,
      'path': path,
      'body': body,
      'label': label,
      'server': account.server,
      'user': account.user,
      'createdAt': DateTime.now().toUtc().toIso8601String(),
    });
  }

  /// Sends a write, or queues it: runs [online] unless writes are already
  /// waiting (they keep their order), and on a network failure, or when
  /// queueing, stores the request and returns [offline]. [offline] runs
  /// first, so input it rejects never reaches the queue; any other server
  /// answer is rethrown.
  Future<T> write<T>({
    required Future<T> Function() online,
    required T Function() offline,
    required String method,
    required String path,
    Object? body,
    required String label,
  }) async {
    final result = offline();
    if ((await pending()).isEmpty) {
      try {
        return await online();
      } on ApiError catch (error) {
        if (!error.isNetwork) rethrow;
      }
    }
    await add(method, path, body: body, label: label);
    return result;
  }

  /// The owner's queued writes, oldest first, stuck ones included.
  Future<List<OutboxOp>> pending() async => _own(await _records(await _open()));

  /// How many writes the owner has waiting.
  Stream<int> count() async* {
    yield* _changes().map((ops) => ops.length);
  }

  /// The owner's writes the server keeps failing on.
  Stream<List<OutboxOp>> stuck() async* {
    yield* _changes().map((ops) => [...ops.where((op) => op.stuck)]);
  }

  /// How many queued writes belong to other accounts or servers.
  Stream<int> others() async* {
    final db = await _open();
    yield* Stores.outbox
        .query()
        .onSnapshots(db)
        .map((records) => records.where((r) => !_owns(r.value)).length);
  }

  /// Gives writes queued before the account was known to [owner]. Call only
  /// when the stored token was just confirmed to belong to [owner].
  Future<void> claim() async {
    final account = owner;
    if (account?.user == null) return;
    final db = await _open();
    for (final r in await Stores.outbox.find(db)) {
      final server = r.value['server'] as String?;
      if (r.value['user'] != null) continue;
      if (server != null && server != account!.server) continue;
      await Stores.outbox.record(r.key).update(db, {
        'server': account!.server,
        'user': account.user,
      });
    }
  }

  /// Discards the owner's queued writes (a sign-out the user asked for).
  Future<void> clear() async {
    final db = await _open();
    for (final op in await pending()) {
      await Stores.outbox.record(op.key).delete(db);
    }
  }

  /// Discards writes that belong to other accounts or servers.
  Future<void> clearOthers() async {
    final db = await _open();
    for (final r in await Stores.outbox.find(db)) {
      if (!_owns(r.value)) await Stores.outbox.record(r.key).delete(db);
    }
  }

  /// Discards one of the owner's writes.
  Future<void> discard(int key) async {
    final db = await _open();
    final record = Stores.outbox.record(key);
    final value = await record.get(db);
    if (value != null && _owns(value)) await record.delete(db);
  }

  /// Gives the owner's stuck writes a fresh start.
  Future<void> unstick() async {
    final db = await _open();
    for (final op in await pending()) {
      if (!op.stuck && op.attempts == 0) continue;
      await Stores.outbox.record(op.key).update(db, {
        'attempts': 0,
        'stuck': false,
        'error': null,
      });
    }
  }

  /// Replays the owner's queued writes in order (see the class comment for
  /// what each answer does).
  Future<FlushResult> flush(ApiClient api) async {
    final db = await _open();
    var sent = 0;
    final rejected = <String>[];
    final held = <String>{};
    for (final op in await pending()) {
      if (op.stuck || op.touches(held)) {
        held.add(op.target);
        continue;
      }
      try {
        await api.send(op.method, op.path, payload: op.body);
        sent++;
      } on ApiError catch (error) {
        if (error.hasEnvelope && _verdicts.contains(error.code)) {
          rejected.add('${op.label}: ${error.message}');
        } else if (error.hasEnvelope && error.status >= 500) {
          final attempts = op.attempts + 1;
          final stuck = attempts >= stuckAfter;
          await Stores.outbox.record(op.key).update(db, {
            'attempts': attempts,
            'stuck': stuck,
            'error': error.message,
          });
          if (!stuck) {
            return FlushResult(sent: sent, rejected: rejected, offline: true);
          }
          held.add(op.target);
          continue;
        } else {
          return FlushResult(
            sent: sent,
            rejected: rejected,
            offline: true,
            retryAfter: error.retryAfter,
          );
        }
      }
      await Stores.outbox.record(op.key).delete(db);
    }
    return FlushResult(sent: sent, rejected: rejected);
  }

  Stream<List<OutboxOp>> _changes() async* {
    final db = await _open();
    yield* Stores.outbox
        .query(finder: Finder(sortOrders: [SortOrder(Field.key)]))
        .onSnapshots(db)
        .map(_own);
  }

  Future<List<RecordSnapshot<int, Map<String, Object?>>>> _records(
    Database db,
  ) => Stores.outbox.find(
    db,
    finder: Finder(sortOrders: [SortOrder(Field.key)]),
  );

  List<OutboxOp> _own(List<RecordSnapshot<int, Map<String, Object?>>> rows) => [
    for (final r in rows)
      if (_owns(r.value))
        OutboxOp(
          key: r.key,
          method: r.value['method'] as String,
          path: r.value['path'] as String,
          label: (r.value['label'] as String?) ?? 'A change',
          body: r.value['body'],
          attempts: (r.value['attempts'] as int?) ?? 0,
          error: r.value['error'] as String?,
          stuck: (r.value['stuck'] as bool?) ?? false,
        ),
  ];

  bool _owns(Map<String, Object?> value) {
    final account = owner;
    return account != null &&
        value['server'] == account.server &&
        value['user'] == account.user;
  }
}
