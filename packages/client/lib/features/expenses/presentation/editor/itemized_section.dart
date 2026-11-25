import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/expenses/presentation/editor/split_section.dart';
import 'package:splitspends/features/friends/presentation/people.dart';

/// Line items assigned to people, plus tax and tip, with live totals.
class ItemizedSection extends ConsumerWidget {
  const ItemizedSection({super.key, required this.form, required this.people});

  final ExpenseForm form;
  final Map<String, User> people;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(form.currency);
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final preview = form.preview;
    final expected = form.itemsTotal + form.tax + form.tip;
    final mismatch = (form.total ?? 0) != expected;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 20, 16, 8),
          child: Text('Split', style: text.titleMedium),
        ),
        SplitMethodPicker(form: form),
        const SizedBox(height: 8),
        for (final item in form.items)
          _ItemCard(key: item.key, form: form, item: item, people: people),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Align(
            alignment: Alignment.centerLeft,
            child: TextButton.icon(
              onPressed: () => form.update(
                () => form.items.add(ItemDraft(userIds: {...form.people})),
              ),
              icon: const Icon(Icons.add_rounded),
              label: const Text('Add item'),
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Row(
            children: [
              Expanded(
                child: AmountField(
                  currency: form.currency,
                  label: 'Tax',
                  initial: form.tax == 0 ? null : form.tax,
                  onChanged: (v) => form.update(() => form.tax = v ?? 0),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: AmountField(
                  currency: form.currency,
                  label: 'Tip',
                  initial: form.tip == 0 ? null : form.tip,
                  onChanged: (v) => form.update(() => form.tip = v ?? 0),
                ),
              ),
            ],
          ),
        ),
        if (mismatch && expected > 0)
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    'Items + tax + tip = ${Money.format(expected, info)}',
                    style: TextStyle(color: scheme.error),
                  ),
                ),
                TextButton(
                  onPressed: form.syncTotalWithItems,
                  child: const Text('Use as total'),
                ),
              ],
            ),
          ),
        Card(
          margin: const EdgeInsets.fromLTRB(16, 12, 16, 0),
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Per person', style: text.labelLarge),
                const SizedBox(height: 6),
                for (final e in preview.owed.entries)
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 3),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(
                            people.nameOf(e.key, meId: form.meId, full: true),
                          ),
                        ),
                        Text(
                          Money.format(e.value, info),
                          style: text.titleSmall,
                        ),
                      ],
                    ),
                  ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _ItemCard extends StatefulWidget {
  const _ItemCard({
    super.key,
    required this.form,
    required this.item,
    required this.people,
  });

  final ExpenseForm form;
  final ItemDraft item;
  final Map<String, User> people;

  @override
  State<_ItemCard> createState() => _ItemCardState();
}

class _ItemCardState extends State<_ItemCard> {
  late final _name = TextEditingController(text: widget.item.name);

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final form = widget.form;
    final item = widget.item;
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 0, 16, 10),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 12, 4, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  flex: 3,
                  child: TextField(
                    controller: _name,
                    textCapitalization: TextCapitalization.sentences,
                    onChanged: (v) => item.name = v,
                    decoration: const InputDecoration(
                      isDense: true,
                      hintText: 'Item',
                    ),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  flex: 2,
                  // Follows the name as it is typed, so a screen reader
                  // says which item the price belongs to.
                  child: ValueListenableBuilder(
                    valueListenable: _name,
                    builder: (_, name, _) => AmountField(
                      currency: form.currency,
                      initial: item.amount == 0 ? null : item.amount,
                      semanticLabel: name.text.trim().isEmpty
                          ? 'Price of the item'
                          : 'Price of ${name.text.trim()}',
                      onChanged: (v) => form.update(() => item.amount = v ?? 0),
                    ),
                  ),
                ),
                IconButton(
                  tooltip: 'Remove item',
                  icon: const Icon(Icons.close_rounded),
                  onPressed: () => form.update(() => form.items.remove(item)),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [
                for (final id in form.people)
                  FilterChip(
                    avatar: widget.people[id] == null
                        ? null
                        : UserAvatar(user: widget.people[id]!, size: 22),
                    label: Text(widget.people.nameOf(id, meId: form.meId)),
                    selected: item.userIds.contains(id),
                    showCheckmark: false,
                    onSelected: (v) => form.update(
                      () => v ? item.userIds.add(id) : item.userIds.remove(id),
                    ),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
