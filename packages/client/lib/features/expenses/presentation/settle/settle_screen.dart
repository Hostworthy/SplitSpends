import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';
import 'package:uuid/uuid.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/widgets/back_home_button.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/expenses/domain/suggestions.dart';
import 'package:splitspends/features/friends/domain/friend.dart';
import 'package:splitspends/features/friends/presentation/people.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

/// Record a payment between two people (settle up).
class SettleScreen extends ConsumerStatefulWidget {
  const SettleScreen({super.key, required this.args});

  final SettleArgs args;

  @override
  ConsumerState<SettleScreen> createState() => _SettleScreenState();
}

class _SettleScreenState extends ConsumerState<SettleScreen> {
  String? _from;
  String? _to;
  int? _amount;

  /// The currency of a chosen suggestion or of [SettleArgs]; otherwise the
  /// group's (once loaded) or yours, see [_currencyFor].
  String? _currency;
  DateTime _date = Dates.today();
  final _notes = TextEditingController();
  bool _saving = false;
  int _amountVersion = 0;

  /// The payment's id, chosen once: recording again after an error whose
  /// request may have reached the server cannot record it twice.
  final String _id = const Uuid().v4();

  @override
  void initState() {
    super.initState();
    final me = ref.read(meProvider);
    _from = widget.args.fromUserId ?? me?.id;
    _to = widget.args.toUserId;
    _amount = widget.args.amount;
    _currency = widget.args.currency;
  }

  /// Null while the group is still loading: a payment in a group is in the
  /// group's currency, and guessing another one would book it apart from
  /// the debt it settles.
  String? _currencyFor(Group? group, User? me) {
    if (_currency != null) return _currency;
    if (widget.args.groupId != null) return group?.defaultCurrency;
    return me?.defaultCurrency ?? 'USD';
  }

  @override
  void dispose() {
    _notes.dispose();
    super.dispose();
  }

  void _suggest(Debt debt) {
    HapticFeedback.selectionClick();
    setState(() {
      _from = debt.from;
      _to = debt.to;
      _amount = debt.amount;
      _currency = debt.currency;
      _amountVersion++;
    });
  }

  Future<void> _save() async {
    if (_from == null || _to == null || _from == _to) {
      showMessage(context, 'Choose who paid whom');
      return;
    }
    if (_amount == null || _amount! <= 0) {
      showMessage(context, 'Enter an amount');
      return;
    }
    final groupId = widget.args.groupId;
    final currency = _currencyFor(
      groupId == null ? null : ref.read(groupProvider(groupId)).value,
      ref.read(meProvider),
    );
    if (currency == null) return;
    // The payment outlives this screen when the person closes it
    // meanwhile; the lists must refresh either way.
    final container = ProviderScope.containerOf(context, listen: false);
    setState(() => _saving = true);
    try {
      await ref
          .read(expensesRepositoryProvider)
          .recordPayment(
            id: _id,
            groupId: widget.args.groupId,
            fromUserId: _from!,
            toUserId: _to!,
            amount: _amount!,
            currency: currency,
            date: Dates.toApi(_date),
            notes: _notes.text.trim().isEmpty ? null : _notes.text.trim(),
          );
      unawaited(HapticFeedback.heavyImpact());
      refreshLedgerIn(container);
      if (!mounted) return;
      context.pop();
      showMessage(context, 'Payment recorded 🎉');
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<String?> _pickPerson(List<User> candidates, String? exclude) {
    final meId = ref.read(meProvider)?.id;
    return showModalBottomSheet<String>(
      useRootNavigator: true,
      context: context,
      builder: (context) => SafeArea(
        child: ListView(
          shrinkWrap: true,
          children: [
            for (final u in candidates)
              if (u.id != exclude)
                ListTile(
                  leading: UserAvatar(user: u),
                  title: Text(u.id == meId ? 'You' : u.displayName),
                  onTap: () => Navigator.pop(context, u.id),
                ),
          ],
        ),
      ),
    );
  }

  Future<void> _choosePayer(List<User> candidates) async {
    final id = await _pickPerson(candidates, _to);
    if (id != null) setState(() => _from = id);
  }

  Future<void> _choosePayee(List<User> candidates) async {
    final id = await _pickPerson(candidates, _from);
    if (id != null) setState(() => _to = id);
  }

  void _swap() => setState(() {
    final from = _from;
    _from = _to;
    _to = from;
  });

  Future<void> _chooseDate() async {
    final range = Dates.pickable(_date);
    final picked = await showDatePicker(
      context: context,
      initialDate: _date,
      firstDate: range.first,
      lastDate: range.last,
    );
    if (picked != null) setState(() => _date = picked);
  }

  /// Payments that would settle the group's debts, or your balances with
  /// friends (one friend when the screen was opened for them).
  List<Debt> _suggested(User? me) {
    if (me == null) return const [];
    final groupId = widget.args.groupId;
    if (groupId != null) {
      return suggestions(
        meId: me.id,
        debts: ref.watch(groupBalancesProvider(groupId)).value?.debts ?? [],
      );
    }
    return suggestions(
      meId: me.id,
      friends: ref.watch(friendsProvider).value ?? [],
      friendId: widget.args.friendId,
    );
  }

  @override
  Widget build(BuildContext context) {
    final me = ref.watch(meProvider);
    final groupId = widget.args.groupId;
    final groupState = groupId == null
        ? null
        : ref.watch(groupProvider(groupId));
    final group = groupState?.value;
    final candidates =
        group?.members ??
        [
          ?me,
          for (final f in ref.watch(friendsProvider).value ?? const <Friend>[])
            f.user,
        ];
    final text = Theme.of(context).textTheme;
    final currency = _currencyFor(group, me);

    final suggested = _suggested(me);
    return Scaffold(
      appBar: AppBar(
        leading: const BackHomeButton(close: true),
        title: Text(group == null ? 'Settle up' : 'Settle up · ${group.name}'),
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          MaxWidth(
            width: 560,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (suggested.isNotEmpty) ...[
                  _Suggestions(
                    debts: suggested,
                    avatars: groupId == null,
                    onPick: _suggest,
                  ),
                  const SizedBox(height: 16),
                ],
                _Parties(
                  from: _from,
                  to: _to,
                  group: group,
                  onFrom: () => _choosePayer(candidates),
                  onTo: () => _choosePayee(candidates),
                  onSwap: _swap,
                ),
                const SizedBox(height: 16),
                // Payments use the debt's currency: the group's, or yours
                // outside groups. There is no per-payment currency choice.
                if (currency == null && groupState!.hasError)
                  ErrorView(
                    error: groupState.error!,
                    onRetry: () => ref.invalidate(groupProvider(groupId!)),
                  )
                else if (currency == null)
                  const Center(child: CircularProgressIndicator())
                else
                  AmountField(
                    key: ValueKey('settle-$currency-$_amountVersion'),
                    currency: currency,
                    initial: _amount,
                    large: true,
                    autofocus: _amount == null,
                    onSubmitted: _save,
                    onChanged: (v) => _amount = v,
                  ),
                const SizedBox(height: 12),
                Align(
                  alignment: Alignment.centerLeft,
                  child: ActionChip(
                    avatar: const Icon(Icons.event_rounded, size: 18),
                    label: Text(Dates.short(_date)),
                    onPressed: _chooseDate,
                  ),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _notes,
                  decoration: const InputDecoration(
                    labelText: 'Note (optional)',
                    hintText: 'Bank transfer, cash…',
                  ),
                ),
                const SizedBox(height: 24),
                FilledButton.icon(
                  onPressed: _saving || currency == null ? null : _save,
                  icon: const Icon(Icons.check_rounded),
                  label: const Text('Record payment'),
                ),
                const SizedBox(height: 8),
                Text(
                  'Split Spends only records the payment. Send the money with your usual app.',
                  style: text.bodySmall,
                  textAlign: TextAlign.center,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Chips for [debts]; a chip fills in the payment it would make.
class _Suggestions extends ConsumerWidget {
  const _Suggestions({
    required this.debts,
    required this.avatars,
    required this.onPick,
  });

  final List<Debt> debts;

  /// Whether each chip shows the other person, as when the debts come from
  /// several friends rather than one group.
  final bool avatars;
  final ValueChanged<Debt> onPick;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final meId = ref.watch(meProvider)?.id;
    final people = ref.watch(peopleProvider);
    final config = ref.watch(configValueProvider);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Suggested', style: Theme.of(context).textTheme.labelLarge),
        const SizedBox(height: 8),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final d in debts)
              ActionChip(
                avatar: switch (people[d.from == meId ? d.to : d.from]) {
                  final User other when avatars => UserAvatar(
                    user: other,
                    size: 22,
                  ),
                  _ => null,
                },
                label: Text(
                  '${people.name(d.from, meId: meId)} → ${people.name(d.to, meId: meId)} · '
                  '${Money.format(d.amount, config.currency(d.currency))}',
                ),
                onPressed: () => onPick(d),
              ),
          ],
        ),
      ],
    );
  }
}

/// Who paid whom, with a button to swap them.
class _Parties extends StatelessWidget {
  const _Parties({
    required this.from,
    required this.to,
    required this.group,
    required this.onFrom,
    required this.onTo,
    required this.onSwap,
  });

  final String? from;
  final String? to;
  final Group? group;
  final VoidCallback onFrom;
  final VoidCallback onTo;
  final VoidCallback onSwap;

  @override
  Widget build(BuildContext context) => Card(
    child: Padding(
      padding: const EdgeInsets.all(8),
      child: Row(
        children: [
          _PersonPicker(
            id: from,
            label: 'Paid by',
            group: group,
            onTap: onFrom,
          ),
          IconButton.filledTonal(
            tooltip: 'Swap',
            onPressed: onSwap,
            icon: const Icon(Icons.swap_horiz_rounded),
          ),
          _PersonPicker(id: to, label: 'Paid to', group: group, onTap: onTo),
        ],
      ),
    ),
  );
}

/// One side of the payment, who paid or who was paid, taking its share of
/// the row. Tapping picks someone else.
class _PersonPicker extends ConsumerWidget {
  const _PersonPicker({
    required this.id,
    required this.label,
    required this.group,
    required this.onTap,
  });

  final String? id;
  final String label;

  /// Where to find [id] when the person is not otherwise known.
  final Group? group;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final people = ref.watch(peopleProvider);
    final meId = ref.watch(meProvider)?.id;
    final text = Theme.of(context).textTheme;
    final id = this.id;
    final user = id == null ? null : people[id] ?? group?.member(id);
    return Expanded(
      child: InkWell(
        borderRadius: BorderRadius.circular(20),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 12),
          child: Column(
            children: [
              user == null
                  ? const CircleAvatar(
                      radius: 32,
                      child: Icon(Icons.person_add_alt_1_rounded),
                    )
                  : UserAvatar(user: user, size: 64),
              const SizedBox(height: 8),
              Text(label, style: text.labelMedium),
              Text(
                id == null ? 'Choose' : people.name(id, meId: meId, full: true),
                style: text.titleMedium,
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
