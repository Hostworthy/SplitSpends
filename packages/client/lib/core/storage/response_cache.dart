import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/storage/local_db.dart';

/// Keeps the last successful GET responses in sembast for offline reads.
///
/// sembast holds the whole database in memory, so only the [limit] most
/// recently written responses are kept.
class SembastResponseCache implements ResponseCache {
  SembastResponseCache(
    this._open, {
    this.limit = 300,
    DateTime Function()? clock,
  }) : _clock = clock ?? DateTime.now;

  final Future<Database> Function() _open;
  final DateTime Function() _clock;
  final int limit;

  @override
  Future<Object?> read(String key) async =>
      (await Stores.cache.record(key).get(await _open()))?['body'];

  @override
  Future<void> write(String key, Object? body) async {
    try {
      final db = await _open();
      await db.transaction((txn) async {
        await Stores.cache.record(key).put(txn, {
          'body': body,
          'at': _clock().toUtc().toIso8601String(),
        });
        if (await Stores.cache.count(txn) <= limit) return;
        final stale = await Stores.cache.findKeys(
          txn,
          finder: Finder(sortOrders: [SortOrder('at', false)], offset: limit),
        );
        await Stores.cache.records(stale).delete(txn);
      });
    } catch (_) {
      // Caching is best effort.
    }
  }

  @override
  Future<void> clear() async => Stores.cache.delete(await _open());
}
