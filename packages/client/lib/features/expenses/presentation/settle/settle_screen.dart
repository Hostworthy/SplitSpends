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
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
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

  void _suggest(String from, String to, int amount, String currency) {
    HapticFeedback.selectionClick();
    setState(() {
      _from = from;
      _to = to;
      _amount = amount;
      _currency = currency;
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

  @override
  Widget build(BuildContext context) {
    final me = ref.watch(meProvider);
    final people = ref.watch(peopleProvider);
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
    final config = ref.watch(configValueProvider);
    final text = Theme.of(context).textTheme;
    final currency = _currencyFor(group, me);

    final suggestions = <Widget>[];
    if (groupId != null) {
      final debts =
          ref.watch(groupBalancesProvider(groupId)).value?.debts ?? const [];
      for (final d in debts.where((d) => d.from == me?.id || d.to == me?.id)) {
        suggestions.add(
          ActionChip(
            label: Text(
              '${people.nameOf(d.from, meId: me?.id)} → ${people.nameOf(d.to, meId: me?.id)} · '
              '${Money.format(d.amount, config.currency(d.currency))}',
            ),
            onPressed: () => _suggest(d.from, d.to, d.amount, d.currency),
          ),
        );
      }
    } else if (me != null) {
      for (final f in ref.watch(friendsProvider).value ?? const <Friend>[]) {
        if (widget.args.friendId != null && f.user.id != widget.args.friendId) {
          continue;
        }
        for (final b in f.balances) {
          final from = b.amount > 0 ? f.user.id : me.id;
          final to = b.amount > 0 ? me.id : f.user.id;
          suggestions.add(
            ActionChip(
              avatar: UserAvatar(user: f.user, size: 22),
              label: Text(
                '${people.nameOf(from, meId: me.id)} → ${people.nameOf(to, meId: me.id)} · '
                '${Money.format(b.amount.abs(), config.currency(b.currency))}',
              ),
              onPressed: () => _suggest(from, to, b.amount.abs(), b.currency),
            ),
          );
        }
      }
    }

    Widget person(String? id, String label, VoidCallback onTap) {
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
                  id == null
                      ? 'Choose'
                      : people.nameOf(id, meId: me?.id, full: true),
                  style: text.titleMedium,
                  textAlign: TextAlign.center,
                ),
              ],
            ),
          ),
        ),
      );
    }

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
                if (suggestions.isNotEmpty) ...[
                  Text('Suggested', style: text.labelLarge),
                  const SizedBox(height: 8),
                  Wrap(spacing: 8, runSpacing: 8, children: suggestions),
                  const SizedBox(height: 16),
                ],
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(8),
                    child: Row(
                      children: [
                        person(_from, 'Paid by', () async {
                          final id = await _pickPerson(candidates, _to);
                          if (id != null) setState(() => _from = id);
                        }),
                        IconButton.filledTonal(
                          tooltip: 'Swap',
                          onPressed: () => setState(() {
                            final t = _from;
                            _from = _to;
                            _to = t;
                          }),
                          icon: const Icon(Icons.swap_horiz_rounded),
                        ),
                        person(_to, 'Paid to', () async {
                          final id = await _pickPerson(candidates, _from);
                          if (id != null) setState(() => _to = id);
                        }),
                      ],
                    ),
                  ),
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
                    onPressed: () async {
                      final range = Dates.pickable(_date);
                      final picked = await showDatePicker(
                        context: context,
                        initialDate: _date,
                        firstDate: range.first,
                        lastDate: range.last,
                      );
                      if (picked != null) setState(() => _date = picked);
                    },
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
