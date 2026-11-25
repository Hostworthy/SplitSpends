import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/categories/domain/catalog.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';

/// On-device custom categories with the server's rules. Deleting keeps the
/// record (flagged) so expenses keep a valid id and the import can carry it.
class LocalCategoriesRepository implements CategoriesRepository {
  LocalCategoriesRepository(this._ledger);

  final LocalLedger _ledger;

  /// Same cap as the server.
  static const maxCategories = 200;

  static Category _view(JsonMap c) => Category(
    id: c['id'] as String,
    name: c['name'] as String,
    group: (c['group'] as String?) ?? generalCategory.group,
    icon: c['icon'] as String?,
    custom: true,
    mine: true,
  );

  List<String> get _groups => CategoryCatalog(localConfig.categories).groups;

  CategoryDraft _checked(CategoryDraft draft) {
    final problem = draft.problem(_groups);
    if (problem != null) {
      throw ApiError.validation(problem.field, problem.message);
    }
    return draft..name = draft.cleanName;
  }

  @override
  Future<List<Category>> list() async {
    final custom = [
      for (final c in await _ledger.customCategories())
        if (c['deleted'] != true) _view(c),
    ]..sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
    return [...localConfig.categories, ...custom];
  }

  @override
  Future<Category> create(CategoryInput input) async {
    final given = input.id?.trim().toLowerCase() ?? '';
    final id = given.isEmpty ? 'custom.${_ledger.newId()}' : given;
    if (!customId.hasMatch(id)) {
      throw const ApiError.validation(
        'id',
        'Custom category ids look like custom.<uuid>.',
      );
    }
    final draft = _checked(
      CategoryDraft(
        name: input.name ?? '',
        icon: input.icon,
        group: input.group,
      ),
    );
    final db = await _ledger.db;
    return db.transaction((txn) async {
      final existing = await Stores.categories.record(id).get(txn);
      if (existing != null) return _view(existing);
      final live = (await _ledger.customCategories(txn))
          .where((c) => c['deleted'] != true)
          .length;
      if (live >= maxCategories) {
        throw const ApiError.conflict(
          'You can keep up to $maxCategories categories of your own.',
        );
      }
      final at = _ledger.stamp();
      final record = {
        'id': id,
        'name': draft.name,
        'icon': draft.icon,
        'group': draft.group,
        'deleted': false,
        'createdAt': at,
        'updatedAt': at,
      };
      await Stores.categories.record(id).put(txn, record);
      return _view(record);
    });
  }

  Future<JsonMap> _live(DatabaseClient txn, String id) async {
    final c = await Stores.categories.record(id).get(txn);
    if (c == null || c['deleted'] == true) {
      throw ApiError.notFound('Category');
    }
    return c;
  }

  @override
  Future<Category> update(String id, CategoryInput input) async {
    final db = await _ledger.db;
    return db.transaction((txn) async {
      final c = await _live(txn, id);
      final draft = _checked(
        CategoryDraft(
          name: input.name ?? c['name'] as String,
          icon: input.icon ?? c['icon'] as String?,
          group: input.group ?? c['group'] as String?,
        ),
      );
      final next = {
        ...c,
        'name': draft.name,
        'icon': draft.icon,
        'group': draft.group,
        'updatedAt': _ledger.stamp(),
      };
      await Stores.categories.record(id).put(txn, next);
      return _view(next);
    });
  }

  @override
  Future<void> delete(String id) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final c = await _live(txn, id);
      await Stores.categories.record(id).put(txn, {
        ...c,
        'deleted': true,
        'updatedAt': _ledger.stamp(),
      });
    });
  }
}
