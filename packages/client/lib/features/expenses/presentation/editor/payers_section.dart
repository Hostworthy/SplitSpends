import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/friends/presentation/people.dart';

/// "Paid by …" with a sheet for one or several payers.
class PayersSection extends ConsumerWidget {
  const PayersSection({super.key, required this.form, required this.people});

  final ExpenseForm form;
  final Map<String, User> people;

  Future<void> _edit(BuildContext context) => showModalBottomSheet<void>(
    useRootNavigator: true,
    context: context,
    isScrollControlled: true,
    builder: (_) => _PayersSheet(form: form, people: people),
  );

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final payers = form.payerList;
    final label = !form.multiplePayers
        ? people.name(form.singlePayer, meId: form.meId)
        : '${payers.length} people';
    final payer = people[form.singlePayer];
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
      child: Row(
        children: [
          Text('Paid by', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(width: 12),
          ActionChip(
            key: const Key('payers-chip'),
            avatar: !form.multiplePayers && payer != null
                ? UserAvatar(user: payer, size: 24)
                : const Icon(Icons.group_rounded, size: 18),
            label: Text(label),
            onPressed: () => _edit(context),
          ),
          if (form.payerError != null && form.multiplePayers) ...[
            const SizedBox(width: 8),
            Flexible(
              child: Text(
                form.payerError!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _PayersSheet extends ConsumerStatefulWidget {
  const _PayersSheet({required this.form, required this.people});

  final ExpenseForm form;
  final Map<String, User> people;

  @override
  ConsumerState<_PayersSheet> createState() => _PayersSheetState();
}

class _PayersSheetState extends ConsumerState<_PayersSheet> {
  late bool _multiple = widget.form.multiplePayers;
  late final Map<String, int> _amounts = {...widget.form.payers};

  ExpenseForm get form => widget.form;

  @override
  Widget build(BuildContext context) {
    final info = ref.watch(configValueProvider).currency(form.currency);
    final total = form.total ?? 0;
    final sum = _amounts.values.fold<int>(0, (a, b) => a + b);
    final left = total - sum;
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.only(
          bottom: MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: SegmentedButton<bool>(
                segments: const [
                  ButtonSegment(value: false, label: Text('One person')),
                  ButtonSegment(value: true, label: Text('Several people')),
                ],
                selected: {_multiple},
                onSelectionChanged: (v) => setState(() => _multiple = v.first),
              ),
            ),
            const SizedBox(height: 8),
            Flexible(
              child: ListView(
                shrinkWrap: true,
                children: [
                  for (final id in form.people)
                    if (!_multiple)
                      ListTile(
                        leading: _avatar(id),
                        title: Text(
                          widget.people.name(id, meId: form.meId, full: true),
                        ),
                        trailing: id == form.singlePayer && !form.multiplePayers
                            ? const Icon(Icons.check_rounded)
                            : null,
                        onTap: () {
                          form.setSinglePayer(id);
                          Navigator.pop(context);
                        },
                      )
                    else
                      ListTile(
                        leading: _avatar(id),
                        title: Text(
                          widget.people.name(id, meId: form.meId, full: true),
                        ),
                        trailing: SizedBox(
                          width: 150,
                          child: AmountField(
                            currency: form.currency,
                            initial: _amounts[id],
                            semanticLabel: id == form.meId
                                ? 'Paid by you'
                                : 'Paid by ${widget.people.name(id, full: true)}',
                            onChanged: (v) =>
                                setState(() => _amounts[id] = v ?? 0),
                          ),
                        ),
                      ),
                ],
              ),
            ),
            if (_multiple)
              Padding(
                padding: const EdgeInsets.all(16),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        left == 0
                            ? 'Adds up to ${Money.format(total, info)}'
                            : left > 0
                            ? '${Money.format(left, info)} left'
                            : '${Money.format(-left, info)} too much',
                        style: TextStyle(
                          color: left == 0
                              ? Theme.of(context).colorScheme.primary
                              : Theme.of(context).colorScheme.error,
                        ),
                      ),
                    ),
                    FilledButton(
                      onPressed: () {
                        form.setMultiplePayers(
                          Map.fromEntries(
                            _amounts.entries.where((e) => e.value != 0),
                          ),
                        );
                        Navigator.pop(context);
                      },
                      child: const Text('Done'),
                    ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _avatar(String id) {
    final user = widget.people[id];
    return user == null
        ? const CircleAvatar(child: Icon(Icons.person))
        : UserAvatar(user: user);
  }
}
