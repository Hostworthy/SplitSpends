import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends_ledger/rational.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/split_calculator.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';

/// Method picker shared by the split and itemized sections.
class SplitMethodPicker extends StatelessWidget {
  const SplitMethodPicker({super.key, required this.form});

  final ExpenseForm form;

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: SegmentedButton<SplitMethod>(
        showSelectedIcon: false,
        segments: [
          for (final m in SplitMethod.values)
            ButtonSegment(value: m, label: Text(m.label)),
        ],
        selected: {form.method},
        onSelectionChanged: (v) {
          HapticFeedback.selectionClick();
          form.setMethod(v.first);
        },
      ),
    );
  }
}

/// Per-person split for every method except itemized.
class SplitSection extends ConsumerWidget {
  const SplitSection({super.key, required this.form, required this.people});

  final ExpenseForm form;
  final Map<String, User> people;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(form.currency);
    final preview = form.preview;
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;

    String? remaining;
    if (form.method == SplitMethod.exact && (form.total ?? 0) > 0) {
      final r = preview.remaining;
      remaining = r == 0
          ? 'All assigned'
          : r > 0
          ? '${Money.format(r, info)} left'
          : '${Money.format(-r, info)} over';
    } else if (form.method == SplitMethod.percent) {
      remaining = percentLeft(preview.remaining);
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 20, 16, 8),
          child: Row(
            children: [
              Text('Split', style: text.titleMedium),
              const Spacer(),
              if (remaining != null)
                Text(
                  remaining,
                  style: text.labelLarge?.copyWith(
                    color: preview.remaining == 0
                        ? scheme.primary
                        : scheme.error,
                  ),
                ),
            ],
          ),
        ),
        SplitMethodPicker(form: form),
        const SizedBox(height: 8),
        for (final id in form.people)
          _PersonRow(
            key: ValueKey('${form.method.name}-$id'),
            form: form,
            user: people[id],
            userId: id,
            owed: preview.owed[id],
            info: info,
          ),
      ],
    );
  }
}

class _PersonRow extends StatelessWidget {
  const _PersonRow({
    super.key,
    required this.form,
    required this.user,
    required this.userId,
    required this.owed,
    required this.info,
  });

  final ExpenseForm form;
  final User? user;
  final String userId;
  final int? owed;
  final CurrencyInfo info;

  @override
  Widget build(BuildContext context) {
    final name = userId == form.meId ? 'You' : (user?.displayName ?? 'Someone');
    final avatar = user == null
        ? const CircleAvatar(child: Icon(Icons.person))
        : UserAvatar(user: user!);
    final owedText = Text(
      owed == null ? '' : Money.format(owed!, info),
      style: Theme.of(context).textTheme.titleSmall,
    );

    switch (form.method) {
      case SplitMethod.equal:
        final on = form.included.contains(userId);
        return CheckboxListTile(
          value: on,
          secondary: avatar,
          title: Text(name),
          subtitle: on ? owedText : null,
          onChanged: (v) => form.update(() {
            v ?? false
                ? form.included.add(userId)
                : form.included.remove(userId);
          }),
        );
      case SplitMethod.exact:
      case SplitMethod.adjustment:
        return ListTile(
          leading: avatar,
          title: Text(name),
          subtitle: form.method == SplitMethod.adjustment ? owedText : null,
          trailing: SizedBox(
            width: 150,
            child: AmountField(
              currency: form.currency,
              allowNegative: form.method == SplitMethod.adjustment,
              initial: int.tryParse(form.values[userId] ?? ''),
              label: form.method == SplitMethod.adjustment ? '±' : null,
              onChanged: (v) => form.update(() {
                if (v == null) {
                  form.values.remove(userId);
                } else {
                  form.values[userId] = '$v';
                }
              }),
            ),
          ),
        );
      case SplitMethod.percent:
      case SplitMethod.shares:
        return ListTile(
          leading: avatar,
          title: Text(name),
          subtitle: owedText,
          trailing: SizedBox(
            width: 110,
            child: DecimalField(
              initial: form.values[userId],
              suffix: form.method == SplitMethod.percent
                  ? '%'
                  : _sharesLabel(form.values[userId]),
              onChanged: (v) => form.update(() {
                if (v == null) {
                  form.values.remove(userId);
                } else {
                  form.values[userId] = v;
                }
              }),
            ),
          ),
        );
      case SplitMethod.itemized:
        return const SizedBox.shrink();
    }
  }
}

/// A right-aligned decimal input (percentages, shares). Reports dot-decimal
/// text, or null when empty or not a number.
class DecimalField extends StatefulWidget {
  const DecimalField({
    super.key,
    this.initial,
    required this.suffix,
    required this.onChanged,
  });

  final String? initial;
  final String suffix;

  /// Decimal text (dot separator), or null when empty or invalid.
  final ValueChanged<String?> onChanged;

  @override
  State<DecimalField> createState() => _DecimalFieldState();
}

class _DecimalFieldState extends State<DecimalField> {
  late final _controller = TextEditingController(text: widget.initial ?? '');

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => TextField(
    controller: _controller,
    keyboardType: const TextInputType.numberWithOptions(decimal: true),
    inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]'))],
    textAlign: TextAlign.end,
    onChanged: (v) => widget.onChanged(_decimal(v)),
    decoration: InputDecoration(isDense: true, suffixText: widget.suffix),
  );
}

/// Normalizes user text to dot-decimal; null when it is not a number.
String? _decimal(String input) {
  final text = input.trim().replaceAll(',', '.');
  return Rational.tryParseDecimal(text) == null ? null : text;
}

/// "1 share", "2 shares", "0.5 shares".
String _sharesLabel(String? value) {
  final n = num.tryParse(value ?? '1');
  return n == 1 ? 'share' : 'shares';
}
