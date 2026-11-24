import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/widgets/category_icon.dart';
import 'package:splitspends/features/categories/presentation/category_dialog.dart';
import 'package:splitspends/features/categories/presentation/providers.dart';

/// Category picker sheet grouped by display group, with your own
/// categories in their groups and an "Add category" action. Returns the id.
Future<String?> pickCategory(BuildContext context, {String? selected}) {
  return showModalBottomSheet<String>(
    useRootNavigator: true,
    context: context,
    isScrollControlled: true,
    builder: (context) => _CategorySheet(selected: selected),
  );
}

class _CategorySheet extends ConsumerWidget {
  const _CategorySheet({this.selected});

  final String? selected;

  Future<void> _add(BuildContext context) async {
    final created = await showCategoryDialog(context);
    if (created != null && context.mounted) {
      Navigator.pop(context, created.id);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final catalog = ref.watch(categoryCatalogProvider);
    final groups = <String, List<Category>>{
      for (final g in catalog.groups) g: [],
    };
    for (final c in catalog.choices(selected: selected)) {
      groups.putIfAbsent(c.group, () => []).add(c);
    }
    groups.removeWhere((_, list) => list.isEmpty);
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      height: MediaQuery.sizeOf(context).height * 0.75,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 12, 0),
            child: Row(
              children: [
                Expanded(child: Text('Category', style: text.titleMedium)),
                TextButton.icon(
                  key: const Key('add-category'),
                  onPressed: () => _add(context),
                  icon: const Icon(Icons.add_rounded),
                  label: const Text('Add category'),
                ),
              ],
            ),
          ),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
              children: [
                for (final entry in groups.entries) ...[
                  Padding(
                    padding: const EdgeInsets.fromLTRB(4, 16, 4, 8),
                    child: Text(
                      entry.key,
                      style: text.titleSmall?.copyWith(color: scheme.primary),
                    ),
                  ),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      for (final c in entry.value)
                        ChoiceChip(
                          selected: c.id == selected,
                          avatar: Icon(
                            categoryVisual(
                              c.id,
                              icon: c.icon,
                              group: c.group,
                            ).icon,
                            size: 18,
                          ),
                          label: Text(c.name),
                          onSelected: (_) => Navigator.pop(context, c.id),
                        ),
                    ],
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}
