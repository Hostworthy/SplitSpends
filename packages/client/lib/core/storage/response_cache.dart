import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/storage/local_db.dart';

/// Keeps the last successful GET responses in sembast for offline reads.
class SembastResponseCache implements ResponseCache {
  SembastResponseCache(this._open);

  final Future<Database> Function() _open;

  @override
  Future<Object?> read(String key) async =>
      (await Stores.cache.record(key).get(await _open()))?['body'];

  @override
  Future<void> write(String key, Object? body) async {
    try {
      await Stores.cache.record(key).put(await _open(), {
        'body': body,
        'at': DateTime.now().toUtc().toIso8601String(),
      });
    } catch (_) {
      // Caching is best effort.
    }
  }

  @override
  Future<void> clear() async => Stores.cache.delete(await _open());
}
