import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/categories/data/local_categories_repository.dart';
import 'package:splitspends/features/categories/data/remote_categories_repository.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';

final categoriesRepositoryProvider = Provider<CategoriesRepository>(
  (ref) => byMode(
    ref,
    remote: RemoteCategoriesRepository.new,
    local: () => LocalCategoriesRepository(ref.watch(localLedgerProvider)),
  ),
);
