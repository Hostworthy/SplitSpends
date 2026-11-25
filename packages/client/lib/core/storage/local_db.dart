import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/storage/db_factory.dart';

/// Store layout of the on-device database.
///
/// Local mode keeps its data in `meta`, `people`, `groups`, `expenses`,
/// `activity` and `categories` (records shaped like the API's). Connected
/// mode uses `cache` for offline reads and `outbox` for writes waiting to
/// sync.
abstract final class Stores {
  static final meta = StoreRef<String, Object?>('meta');
  static final people = stringMapStoreFactory.store('people');
  static final groups = stringMapStoreFactory.store('groups');
  static final expenses = stringMapStoreFactory.store('expenses');
  static final activity = stringMapStoreFactory.store('activity');

  /// Custom categories (ids are `custom.<uuid>`), each
  /// `{ id, name, icon, group, deleted, createdAt, updatedAt }`.
  static final categories = stringMapStoreFactory.store('categories');

  static final cache = stringMapStoreFactory.store('cache');
  static final outbox = intMapStoreFactory.store('outbox');

  /// Stores that make up the on-device ledger (backup, import, wipe).
  static final ledger = [people, groups, expenses, activity, categories];
}

/// Version of the record shapes in the on-device database. Every release
/// so far wrote version 1 (sembast's default). When a stored shape changes,
/// raise it and add the step to [migrate].
const schemaVersion = 1;

/// Opens [path] with [factory] at [schemaVersion], migrating older data.
Future<Database> openVersioned(DatabaseFactory factory, String path) => factory
    .openDatabase(path, version: schemaVersion, onVersionChanged: migrate);

/// Brings records written at [from] up to [to] inside the open transaction.
/// A new database arrives with [from] 0 and needs no step.
Future<void> migrate(Database db, int from, int to) async {
  if (from > to) {
    throw const ApiError.conflict(
      'The data on this device was saved by a newer version of Split '
      'Spends. Update the app to open it.',
    );
  }
}

/// Opens the database once; tests override it with an in-memory one.
final databaseProvider = FutureProvider<Database>(
  (ref) => openAppDatabase('splitspends.db'),
);
