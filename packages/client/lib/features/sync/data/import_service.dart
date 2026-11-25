import 'package:http/http.dart' as http;
import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/sync/domain/import_payload.dart';

/// Moves on-device data to an account (POST /api/v1/import). The local copy
/// stays until the user removes it.
class ImportService {
  ImportService(this._ledger, this._http);

  final LocalLedger _ledger;
  final http.Client _http;

  Future<bool> hasProfile() async =>
      await Stores.meta.record('profile').get(await _ledger.db) != null;

  Future<ImportSummary> summary() async {
    final db = await _ledger.db;
    if (!await hasProfile()) return const ImportSummary();
    final expenses = await _ledger.expenses(db);
    final categories = await _ledger.customCategories(db);
    return ImportSummary(
      groups: (await _ledger.groups(db)).length,
      expenses: expenses.where((e) => e['deleted'] != true).length,
      people: (await _ledger.people(db)).length,
      categories: categories.where((c) => c['deleted'] != true).length,
      uploaded: _uploaded(await Stores.meta.record(_uploadedKey).get(db)),
    );
  }

  /// Where the data went on its last upload; cleared with the local copy.
  static const _uploadedKey = 'importedTo';

  static ({String server, DateTime at})? _uploaded(Object? raw) {
    if (raw is! Map) return null;
    final server = raw['serverUrl'];
    final at = DateTime.tryParse('${raw['at']}');
    if (server is! String || at == null) return null;
    return (server: server, at: at);
  }

  Future<JsonMap> payload() async {
    final db = await _ledger.db;
    return buildImportRequest(
      meId: (await _ledger.me(db)).id,
      people: (await _ledger.people(db)).values.toList(),
      groups: await _ledger.groups(db),
      expenses: await _ledger.expenses(db),
      categories: await _ledger.customCategories(db),
    );
  }

  /// Uploads everything, then rewrites local ids with the returned map.
  Future<void> upload({
    required String serverUrl,
    required String token,
  }) async {
    final api = ApiClient(
      serverUrl: serverUrl,
      token: token,
      httpClient: _http,
      timeout: const Duration(minutes: 2),
    );
    final body = await payload();
    final map = await api.postData(
      '/import',
      body,
      (d) => ((d as JsonMap)['people'] as Map? ?? const {}).map(
        (k, v) => MapEntry(k as String, v as String),
      ),
    );
    await _apply(map, serverUrl);
  }

  Future<void> _apply(Map<String, String> map, String serverUrl) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final mapped = applyPeopleMap(
        map,
        people: (await _ledger.people(txn)).values.toList(),
        groups: await _ledger.groups(txn),
        expenses: await _ledger.expenses(txn),
        activity: [for (final r in await Stores.activity.find(txn)) r.value],
      );
      await Stores.people.delete(txn);
      for (final p in mapped.people) {
        await Stores.people.record(p['id'] as String).put(txn, p);
      }
      for (final g in mapped.groups) {
        await Stores.groups.record(g['id'] as String).put(txn, g);
      }
      for (final e in mapped.expenses) {
        await Stores.expenses.record(e['id'] as String).put(txn, e);
      }
      for (final a in mapped.activity) {
        await Stores.activity.record(a['id'] as String).put(txn, a);
      }
      final profile = await Stores.meta.record('profile').get(txn) as JsonMap?;
      if (profile != null) {
        final id = profile['id'] as String;
        await Stores.meta.record('profile').put(txn, {
          ...profile,
          'id': map[id] ?? id,
        });
      }
      await Stores.meta.record(_uploadedKey).put(txn, {
        'serverUrl': serverUrl,
        'at': _ledger.stamp(),
      });
    });
  }

  /// Deletes the on-device ledger and profile (explicit user action).
  Future<void> removeLocalCopy() async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      for (final store in Stores.ledger) {
        await store.delete(txn);
      }
      await Stores.meta.record('profile').delete(txn);
      await Stores.meta.record(_uploadedKey).delete(txn);
    });
  }
}
