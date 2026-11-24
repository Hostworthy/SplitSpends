import 'dart:convert';
import 'dart:typed_data';

import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';

const backupFormat = 'splitspends-backup';
const backupVersion = 1;

/// Exports and restores the on-device ledger as one JSON file.
class BackupService {
  BackupService(this._ledger);

  final LocalLedger _ledger;

  Future<Uint8List> export() async {
    final db = await _ledger.db;
    Future<List<JsonMap>> all(StoreRef<String, JsonMap> store) async => [
      for (final r in await store.find(db)) r.value,
    ];
    final body = {
      'format': backupFormat,
      'version': backupVersion,
      'exportedAt': _ledger.stamp(),
      'profile': await Stores.meta.record('profile').get(db),
      'people': await all(Stores.people),
      'groups': await all(Stores.groups),
      'expenses': await all(Stores.expenses),
      'activity': await all(Stores.activity),
      'categories': await all(Stores.categories),
    };
    return Uint8List.fromList(
      utf8.encode(const JsonEncoder.withIndent('  ').convert(body)),
    );
  }

  /// Replaces all on-device data with the backup in [bytes].
  Future<void> restore(Uint8List bytes) async {
    final JsonMap body;
    try {
      body = jsonDecode(utf8.decode(bytes)) as JsonMap;
    } catch (_) {
      throw const ApiError.validation(
        'file',
        'This is not a SplitSpends backup.',
      );
    }
    if (body['format'] != backupFormat || body['profile'] is! Map) {
      throw const ApiError.validation(
        'file',
        'This is not a SplitSpends backup.',
      );
    }
    if ((body['version'] as int? ?? 0) > backupVersion) {
      throw const ApiError.validation(
        'file',
        'This backup is from a newer version of the app.',
      );
    }
    List<JsonMap> rows(String key) => [
      for (final r in (body[key] as List? ?? const []))
        (r as Map).cast<String, Object?>(),
    ];
    final db = await _ledger.db;
    await db.transaction((txn) async {
      for (final store in Stores.ledger) {
        await store.delete(txn);
      }
      Future<void> put(StoreRef<String, JsonMap> store, String key) async {
        for (final r in rows(key)) {
          await store.record(r['id'] as String).put(txn, r);
        }
      }

      await put(Stores.people, 'people');
      await put(Stores.groups, 'groups');
      await put(Stores.expenses, 'expenses');
      await put(Stores.activity, 'activity');
      await put(Stores.categories, 'categories');
      await Stores.meta
          .record('profile')
          .put(txn, (body['profile'] as Map).cast<String, Object?>());
    });
  }
}
