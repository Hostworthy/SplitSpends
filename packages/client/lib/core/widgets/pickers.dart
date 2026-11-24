import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/config_provider.dart';

/// Searchable currency picker sheet. Returns the chosen code.
Future<String?> pickCurrency(BuildContext context, {String? selected}) {
  return showModalBottomSheet<String>(
    useRootNavigator: true,
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
