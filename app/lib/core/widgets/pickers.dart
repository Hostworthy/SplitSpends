import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/widgets/category_icon.dart';

/// Searchable currency picker sheet. Returns the chosen code.
Future<String?> pickCurrency(BuildContext context, {String? selected}) {
  return showModalBottomSheet<String>(
    context: context,
    isScrollControlled: true,
    builder: (context) => _CurrencySheet(selected: selected),
  );
}

class _CurrencySheet extends ConsumerStatefulWidget {
  const _CurrencySheet({this.selected});

  final String? selected;

  @override
  ConsumerState<_CurrencySheet> createState() => _CurrencySheetState();
}

class _CurrencySheetState extends ConsumerState<_CurrencySheet> {
  String _query = '';

  @override
  Widget build(BuildContext context) {
    final config = ref.watch(configValueProvider);
    final all = config.currencies.isEmpty
        ? fallbackCodes.map(fallbackCurrency).toList()
        : config.currencies;
    final q = _query.toLowerCase();
    final shown = all
        .where(
          (c) =>
              q.isEmpty ||
              c.code.toLowerCase().contains(q) ||
              c.name.toLowerCase().contains(q),
        )
        .toList();
    return SizedBox(
      height: MediaQuery.sizeOf(context).height * 0.75,
      child: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
            child: TextField(
              autofocus: true,
              decoration: const InputDecoration(
                prefixIcon: Icon(Icons.search_rounded),
                hintText: 'Search currencies',
              ),
              onChanged: (v) => setState(() => _query = v),
            ),
          ),
          Expanded(
            child: ListView.builder(
              itemCount: shown.length,
              itemBuilder: (context, i) {
                final c = shown[i];
                return ListTile(
                  leading: SizedBox(
                    width: 44,
                    child: Text(
                      c.symbol,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                  ),
                  title: Text(c.code),
                  subtitle: Text(c.name),
                  trailing: c.code == widget.selected
                      ? const Icon(Icons.check_rounded)
                      : null,
                  onTap: () => Navigator.pop(context, c.code),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}

/// Category picker sheet grouped by category group. Returns the id.
Future<String?> pickCategory(BuildContext context, {String? selected}) {
  return showModalBottomSheet<String>(
    context: context,
    isScrollControlled: true,
    builder: (context) => _CategorySheet(selected: selected),
  );
}

class _CategorySheet extends ConsumerWidget {
  const _CategorySheet({this.selected});

  final String? selected;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final categories = ref.watch(configValueProvider).categories;
    final groups = <String, List<Category>>{};
    for (final c in categories) {
      groups.putIfAbsent(c.group, () => []).add(c);
    }
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      height: MediaQuery.sizeOf(context).height * 0.75,
      child: categories.isEmpty
          ? const Center(child: Text('No categories configured'))
          : ListView(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
              children: [
                for (final entry in groups.entries) ...[
                  Padding(
                    padding: const EdgeInsets.fromLTRB(4, 16, 4, 8),
                    child: Text(
                      entry.key,
                      style: Theme.of(context).textTheme.titleSmall
                          ?.copyWith(color: scheme.primary),
                    ),
                  ),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      for (final c in entry.value)
                        ChoiceChip(
                          selected: c.id == selected,
                          avatar: Icon(categoryVisual(c.id).icon, size: 18),
                          label: Text(c.name),
                          onSelected: (_) => Navigator.pop(context, c.id),
                        ),
                    ],
                  ),
                ],
              ],
            ),
    );
  }
}
