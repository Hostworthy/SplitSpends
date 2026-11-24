import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/models/json.dart';

/// Body for creating or changing a custom category. Null fields stay
/// unchanged.
class CategoryInput {
  const CategoryInput({this.id, this.name, this.icon, this.group});

  /// Client-chosen id (`custom.<uuid>`) for idempotent creates.
  final String? id;
  final String? name;
  final String? icon;
  final String? group;

  JsonMap toJson() => {
    'id': ?id,
    'name': ?name,
    'icon': ?icon,
    'group': ?group,
  };
}

/// Built-in and custom expense categories. Implemented on-device (local
/// mode) and over HTTP (connected mode).
abstract interface class CategoriesRepository {
  /// Built-ins, your own custom categories and custom categories used by
  /// expenses you can see.
  Future<List<Category>> list();

  Future<Category> create(CategoryInput input);

  Future<Category> update(String id, CategoryInput input);

  /// Hides the category; expenses keep it and show as General.
  Future<void> delete(String id);
}
