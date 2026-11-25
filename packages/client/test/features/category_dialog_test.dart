import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/features/categories/domain/catalog.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';
import 'package:splitspends/features/categories/presentation/category_dialog.dart';

import '../helpers.dart';

/// Fails the first create after it may have reached the server.
class _Flaky implements CategoriesRepository {
  final created = <CategoryInput>[];

  @override
  Future<List<Category>> list() async => localConfig.categories;

  @override
  Future<Category> create(CategoryInput input) async {
    created.add(input);
    if (created.length == 1) {
      throw const ApiError.network('Cannot reach the server.');
    }
    return Category(
      id: input.id!,
      name: input.name!,
      group: input.group!,
      icon: input.icon,
      custom: true,
      mine: true,
    );
  }

  @override
  Future<Category> update(String id, CategoryInput input) =>
      throw UnimplementedError();

  @override
  Future<void> delete(String id) => throw UnimplementedError();
}

void main() {
  testWidgets('saving again after a failure keeps the category id', (
    tester,
  ) async {
    final repo = _Flaky();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [categoriesRepositoryProvider.overrideWithValue(repo)],
        child: MaterialApp(
          home: Scaffold(
            body: Builder(
              builder: (context) => TextButton(
                onPressed: () => showCategoryDialog(context),
                child: const Text('Open'),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('Open'));
    await settle(tester);
    await tester.enterText(find.byKey(const Key('category-name')), 'Climbing');
    await tester.tap(find.text('Add'));
    await settle(tester);
    await tester.tap(find.text('Add'));
    await settle(tester);

    expect(repo.created, hasLength(2));
    final id = repo.created.first.id;
    expect(id, isNotNull);
    expect(customId.hasMatch(id!), isTrue);
    expect(repo.created.last.id, id);
  });
}
