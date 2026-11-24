import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';

/// HTTP implementation used in connected mode.
class RemoteCategoriesRepository implements CategoriesRepository {
  RemoteCategoriesRepository(this._api);

  final ApiClient _api;

  @override
  Future<List<Category>> list() async =>
      (await _api.getList('/categories', Category.fromJson)).items;

  @override
  Future<Category> create(CategoryInput input) => _api.postData(
    '/categories',
    input.toJson(),
    (d) => Category.fromJson(d as JsonMap),
  );

  @override
  Future<Category> update(String id, CategoryInput input) => _api.patchData(
    '/categories/${Uri.encodeComponent(id)}',
    input.toJson(),
    (d) => Category.fromJson(d as JsonMap),
  );

  @override
  Future<void> delete(String id) =>
      _api.delete('/categories/${Uri.encodeComponent(id)}');
}
