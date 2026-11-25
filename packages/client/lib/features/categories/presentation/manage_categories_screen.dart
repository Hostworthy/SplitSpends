import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/features/categories/data/providers.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/widgets/category_icon.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/categories/presentation/category_dialog.dart';
import 'package:splitspends/features/categories/presentation/providers.dart';

/// Account → Categories: create, rename, change the icon of, or delete your
/// own categories.
class ManageCategoriesScreen extends ConsumerWidget {
  const ManageCategoriesScreen({super.key});

  Future<void> _delete(BuildContext context, Category category) async {
    final ok =
        await showDialog<bool>(
          context: context,
          builder: (context) => AlertDialog(
            title: Text('Delete “${category.name}”?'),
            content: const Text(
              'Expenses in this category keep working and show as General.',
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(context, false),
                child: const Text('Cancel'),
              ),
              FilledButton(
                onPressed: () => Navigator.pop(context, true),
                child: const Text('Delete'),
              ),
            ],
          ),
        ) ??
        false;
    if (!ok || !context.mounted) return;
    final container = ProviderScope.containerOf(context, listen: false);
    try {
      await container.read(categoriesRepositoryProvider).delete(category.id);
      container.invalidate(categoriesProvider);
      if (context.mounted) showMessage(context, 'Category deleted');
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final loaded = ref.watch(categoriesProvider);
    final mine = ref.watch(categoryCatalogProvider).mine;
    return Scaffold(
      appBar: AppBar(title: const Text('Categories')),
      floatingActionButton: FloatingActionButton.extended(
        key: const Key('new-category'),
        onPressed: () => showCategoryDialog(context),
        icon: const Icon(Icons.add_rounded),
        label: const Text('Add category'),
      ),
      body: AsyncView(
        value: loaded,
        onRetry: () => ref.invalidate(categoriesProvider),
        data: (_) => MaxWidth(
          child: ListView(
            padding: const EdgeInsets.only(bottom: 96),
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
                child: Text(
                  'Your own categories appear next to the built-in ones when '
                  'you add an expense. Other people in your groups see them '
                  'on the expenses you file under them.',
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
              ),
              if (mine.isEmpty)
                const Padding(
                  padding: EdgeInsets.all(32),
                  child: Center(child: Text('No categories of your own yet')),
                ),
              for (final c in mine)
                ListTile(
                  leading: CategoryIcon(
                    category: c.id,
                    icon: c.icon,
                    group: c.group,
                    size: 40,
                  ),
                  title: Text(c.name),
                  subtitle: Text(c.group),
                  onTap: () => showCategoryDialog(context, existing: c),
                  trailing: IconButton(
                    tooltip: 'Delete',
                    icon: const Icon(Icons.delete_outline_rounded),
                    onPressed: () => _delete(context, c),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
