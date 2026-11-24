import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/storage/local_db.dart';

/// A write waiting to reach the server.
class OutboxOp {
  const OutboxOp({
    required this.key,
    required this.method,
    required this.path,
    required this.label,
    this.body,
  });

  final int key;
  final String method;
  final String path;
  final String label;
  final Object? body;
}

/// Result of one replay pass.
class FlushResult {
  const FlushResult({
    this.sent = 0,
    this.rejected = const [],
    this.offline = false,
  });

  final int sent;

  /// Labels of changes the server refused (dropped: the server wins).
  final List<String> rejected;

  /// True when the server was unreachable and replay stopped.
  final bool offline;
}

/// Persisted queue of writes made while offline. Creates carry client ids,
/// so replaying after a lost response never duplicates anything.
class Outbox {
  Outbox(this._open);

  final Future<Database> Function() _open;

  Future<void> add(
    String method,
    String path, {
    Object? body,
    required String label,
  }) async {
    await Stores.outbox.add(await _open(), {
      'method': method,
      'path': path,
      'body': body,
      'label': label,
      'createdAt': DateTime.now().toUtc().toIso8601String(),
    });
  }

  Future<List<OutboxOp>> pending() async => [
    for (final r in await Stores.outbox.find(
      await _open(),
      finder: Finder(sortOrders: [SortOrder(Field.key)]),
    ))
      OutboxOp(
        key: r.key,
        method: r.value['method'] as String,
        path: r.value['path'] as String,
        label: (r.value['label'] as String?) ?? 'A change',
        body: r.value['body'],
      ),
  ];

  Stream<int> count() async* {
    yield* Stores.outbox.query().onCount(await _open());
  }

  /// Replays queued writes in order. Stops at the first network failure;
  /// drops writes the server rejects (validation, conflict, not found).
  Future<FlushResult> flush(ApiClient api) async {
    final db = await _open();
    var sent = 0;
    final rejected = <String>[];
    for (final op in await pending()) {
      try {
        await api.send(op.method, op.path, payload: op.body);
        sent++;
      } on ApiError catch (error) {
        if (error.isNetwork || error.status >= 500 || error.isUnauthorized) {
          return FlushResult(sent: sent, rejected: rejected, offline: true);
        }
        rejected.add('${op.label}: ${error.message}');
      }
      await Stores.outbox.record(op.key).delete(db);
    }
    return FlushResult(sent: sent, rejected: rejected);
  }
}
