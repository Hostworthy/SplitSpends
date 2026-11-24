import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:sembast/sembast.dart';

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

/// Opens the database once; tests override it with an in-memory one.
final databaseProvider = FutureProvider<Database>(
  (ref) => openAppDatabase('splitspends.db'),
);
