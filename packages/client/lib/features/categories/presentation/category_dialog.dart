import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:uuid/uuid.dart';

import 'package:splitspends/features/categories/data/providers.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/widgets/category_icon.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/categories/domain/catalog.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';
import 'package:splitspends/features/categories/presentation/providers.dart';

/// Creates a custom category, or edits [existing]. Returns the saved one.
Future<Category?> showCategoryDialog(
  BuildContext context, {
  Category? existing,
}) => showDialog<Category>(
  context: context,
  useRootNavigator: true,
  builder: (context) => _CategoryDialog(existing: existing),
);

class _CategoryDialog extends ConsumerStatefulWidget {
  const _CategoryDialog({this.existing});

  final Category? existing;

  @override
  ConsumerState<_CategoryDialog> createState() => _CategoryDialogState();
}

class _CategoryDialogState extends ConsumerState<_CategoryDialog> {
  late final CategoryDraft _draft = CategoryDraft(
    name: widget.existing?.name ?? '',
    icon: widget.existing?.icon,
    group: widget.existing?.group,
  );
  late final _name = TextEditingController(text: _draft.name);

  /// The new category's id, chosen once: saving again after an error whose
  /// request may have reached the server cannot add it twice.
  final String _id = 'custom.${const Uuid().v4()}';
  bool _saving = false;
  bool _touched = false;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _save(List<String> groups) async {
    setState(() => _touched = true);
    if (_draft.problem(groups) != null) return;
    final container = ProviderScope.containerOf(context, listen: false);
    setState(() => _saving = true);
    try {
      final repo = container.read(categoriesRepositoryProvider);
      final input = CategoryInput(
        id: widget.existing == null ? _id : null,
        name: _draft.cleanName,
        icon: _draft.icon,
        group: _draft.group,
      );
      final saved = widget.existing == null
          ? await repo.create(input)
          : await repo.update(widget.existing!.id, input);
      container.invalidate(categoriesProvider);
      if (mounted) Navigator.pop(context, saved);
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final groups = ref.watch(categoryCatalogProvider).groups;
    if (!groups.contains(_draft.group)) _draft.group = groups.last;
    final problem = _draft.problem(groups);
    final scheme = Theme.of(context).colorScheme;
    return AlertDialog(
      title: Text(widget.existing == null ? 'New category' : 'Edit category'),
      content: SizedBox(
        width: 420,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  CategoryIcon(
                    category: 'custom.preview',
                    icon: _draft.icon,
                    group: _draft.group,
                    size: 48,
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: TextField(
                      key: const Key('category-name'),
                      controller: _name,
                      autofocus: true,
                      maxLength: CategoryDraft.maxName,
                      textCapitalization: TextCapitalization.sentences,
                      decoration: InputDecoration(
                        labelText: 'Name',
                        errorText: _touched && _draft.cleanName.isEmpty
                            ? 'Give the category a name'
                            : null,
                      ),
                      onChanged: (v) => setState(() {
                        _draft.name = v;
                        _touched = true;
                      }),
                      onSubmitted: (_) => _save(groups),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              DropdownButtonFormField<String>(
                initialValue: _draft.group,
                decoration: const InputDecoration(labelText: 'Group'),
                items: [
                  for (final g in groups)
                    DropdownMenuItem(value: g, child: Text(g)),
                ],
                onChanged: (v) =>
                    setState(() => _draft.group = v ?? _draft.group),
              ),
              const SizedBox(height: 16),
              Text('Icon', style: Theme.of(context).textTheme.labelLarge),
              const SizedBox(height: 8),
              Wrap(
                spacing: 4,
                runSpacing: 4,
                children: [
                  for (final entry in categoryIcons.entries)
                    IconButton(
                      tooltip: entry.key.replaceAll('_', ' '),
                      isSelected: _draft.icon == entry.key,
                      style: IconButton.styleFrom(
                        backgroundColor: _draft.icon == entry.key
                            ? scheme.primaryContainer
                            : null,
                      ),
                      icon: Icon(
                        entry.value,
                        color: _draft.icon == entry.key
                            ? scheme.onPrimaryContainer
                            : scheme.onSurfaceVariant,
                      ),
                      onPressed: () => setState(() => _draft.icon = entry.key),
                    ),
                ],
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: _saving || (_touched && problem != null)
              ? null
              : () => _save(groups),
          child: Text(widget.existing == null ? 'Add' : 'Save'),
        ),
      ],
    );
  }
}
