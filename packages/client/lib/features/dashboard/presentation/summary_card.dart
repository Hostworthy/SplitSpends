import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/rates/rates.dart';
import 'package:splitspends/core/widgets/skeleton.dart';

/// Hero card with the overall balance and the owed / owing split.
class SummaryCard extends ConsumerWidget {
  const SummaryCard({super.key, required this.summary});

  final BalanceSummary summary;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final scheme = Theme.of(context).colorScheme;
    final text = Theme.of(context).textTheme;
    final view = ref.watch(moneyViewProvider);
    final colors = BalanceColors.of(context);

    String format(Balance b) =>
        view.format(b.amount, b.currency, absolute: true);

    final net = view.combine(summary.balances);
    final headline = summary.isSettled || net.isEmpty
        ? 'All settled up'
        : net
              .map((b) {
                final sign = b.amount > 0 ? '+' : '−';
                return '$sign${format(b)}';
              })
              .join('  ·  ');
    final converted =
        view.rates != null &&
        [...summary.owed, ...summary.owing].any(
          (b) => b.currency != view.currency && view.rates!.has(b.currency),
        );

    return Card(
      color: scheme.primaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Total balance',
              style: text.labelLarge?.copyWith(
                color: scheme.onPrimaryContainer.withValues(alpha: 0.8),
              ),
            ),
            const SizedBox(height: 4),
            Text(
              headline,
              style: text.headlineSmall?.copyWith(
                color: scheme.onPrimaryContainer,
              ),
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: _Side(
                    label: 'You are owed',
                    values: view.combine(summary.owed).map(format).toList(),
                    color: colors.owed,
                    icon: Icons.south_west_rounded,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: _Side(
                    label: 'You owe',
                    values: view.combine(summary.owing).map(format).toList(),
                    color: colors.owe,
                    icon: Icons.north_east_rounded,
                  ),
                ),
              ],
            ),
            if (converted) ...[
              const SizedBox(height: 10),
              Text(
                'Shown in ${view.currency} at today\'s exchange rates. '
                'Each expense keeps its own currency.',
                style: text.bodySmall?.copyWith(
                  color: scheme.onPrimaryContainer.withValues(alpha: 0.75),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _Side extends StatelessWidget {
  const _Side({
    required this.label,
    required this.values,
    required this.color,
    required this.icon,
  });

  final String label;
  final List<String> values;
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final text = Theme.of(context).textTheme;
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: scheme.surface.withValues(alpha: 0.7),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, size: 16, color: color),
              const SizedBox(width: 6),
              Flexible(
                child: Text(
                  label,
                  style: text.labelMedium,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          if (values.isEmpty)
            Text('—', style: text.titleMedium)
          else
            for (final v in values)
              Text(
                v,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: text.titleMedium?.copyWith(
                  color: color,
                  fontWeight: FontWeight.w700,
                ),
              ),
        ],
      ),
    );
  }
}

class SummaryCardSkeleton extends StatelessWidget {
  const SummaryCardSkeleton({super.key});

  @override
  Widget build(BuildContext context) => const Card(
    child: Padding(
      padding: EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Skeleton(width: 100),
          SizedBox(height: 10),
          Skeleton(width: 180, height: 28),
          SizedBox(height: 20),
          Row(
            children: [
              Expanded(child: Skeleton(height: 64, radius: 16)),
              SizedBox(width: 12),
              Expanded(child: Skeleton(height: 64, radius: 16)),
            ],
          ),
        ],
      ),
    ),
  );
}
