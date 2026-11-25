import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/theme/theme.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/money/money.dart';

/// Formats money with the server's currency metadata.
class MoneyText extends ConsumerWidget {
  const MoneyText(
    this.amount,
    this.currency, {
    super.key,
    this.style,
    this.absolute = false,
  });

  final int amount;
  final String currency;
  final TextStyle? style;
  final bool absolute;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(currency);
    return Text(
      Money.format(amount, info, absolute: absolute),
      style: style,
      maxLines: 1,
    );
  }
}

/// "you are owed $12" / "you owe $12" / "settled up", colored by sign.
class BalanceText extends ConsumerWidget {
  const BalanceText({
    super.key,
    required this.balances,
    this.owedLabel = 'you are owed',
    this.oweLabel = 'you owe',
    this.settledLabel = 'settled up',
    this.align = CrossAxisAlignment.end,
    this.dense = false,
  });

  final List<Balance> balances;
  final String owedLabel;
  final String oweLabel;
  final String settledLabel;
  final CrossAxisAlignment align;
  final bool dense;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final colors = BalanceColors.of(context);
    final config = ref.watch(configValueProvider);
    final theme = Theme.of(context).textTheme;
    final nonZero = balances.where((b) => !b.isZero).toList();
    if (nonZero.isEmpty) {
      return Text(
        settledLabel,
        style: theme.bodySmall?.copyWith(color: colors.settled),
      );
    }
    return Column(
      crossAxisAlignment: align,
      mainAxisSize: MainAxisSize.min,
      children: [
        for (final b in nonZero)
          Column(
            crossAxisAlignment: align,
            mainAxisSize: MainAxisSize.min,
            children: [
              if (!dense)
                Text(
                  b.amount > 0 ? owedLabel : oweLabel,
                  style: theme.labelSmall?.copyWith(
                    color: colors.forAmount(b.amount),
                  ),
                ),
              Text(
                Money.format(
                  b.amount,
                  config.currency(b.currency),
                  absolute: true,
                ),
                style: theme.titleSmall?.copyWith(
                  color: colors.forAmount(b.amount),
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
      ],
    );
  }
}

/// Pill showing a single signed amount.
class BalancePill extends ConsumerWidget {
  const BalancePill({super.key, required this.balance});

  final Balance balance;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final colors = BalanceColors.of(context);
    final info = ref.watch(configValueProvider).currency(balance.currency);
    final positive = balance.amount > 0;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: positive ? colors.owedContainer : colors.oweContainer,
        borderRadius: BorderRadius.circular(99),
      ),
      child: Text(
        '${positive ? '+' : '−'}${Money.format(balance.amount, info, absolute: true)}',
        style: Theme.of(context).textTheme.labelLarge?.copyWith(
          color: colors.forAmount(balance.amount),
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}
