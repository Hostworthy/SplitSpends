import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/features/categories/domain/catalog.dart';

/// Built-in plus custom categories for the current mode.
final categoriesProvider = FutureProvider<List<Category>>(
  (ref) => ref.watch(categoriesRepositoryProvider).list(),
);

/// Always available: the loaded categories, or the built-ins from the
/// config while they load or when they cannot be loaded.
final categoryCatalogProvider = Provider<CategoryCatalog>((ref) {
  final loaded = ref.watch(categoriesProvider).value;
  return CategoryCatalog(loaded ?? ref.watch(configValueProvider).categories);
});
