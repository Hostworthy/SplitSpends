import 'package:fl_chart/fl_chart.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/category_icon.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/stats.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

class GroupStatsTab extends ConsumerStatefulWidget {
  const GroupStatsTab({super.key, required this.group});

  final Group group;

  @override
  ConsumerState<GroupStatsTab> createState() => _GroupStatsTabState();
}

class _GroupStatsTabState extends ConsumerState<GroupStatsTab> {
  late final String _currency = widget.group.defaultCurrency;

  @override
  Widget build(BuildContext context) {
    final key = (groupId: widget.group.id, currency: _currency);
    final stats = ref.watch(statsProvider(key));
    return RefreshIndicator(
      onRefresh: () async {
        ref.invalidate(statsProvider(key));
        await ref.read(statsProvider(key).future);
      },
      child: AsyncView(
        value: stats,
        onRetry: () => ref.invalidate(statsProvider(key)),
        data: (s) => ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 96),
          children: [
            Row(
              children: [
                Expanded(
                  child: StatsTotal(stats: s, label: 'Group spending'),
                ),
              ],
            ),
            if (s.isEmpty)
              const EmptyState(
                icon: Icons.insights_rounded,
                title: 'No spending yet',
                message: 'Charts appear once there are expenses.',
              )
            else ...[
              const SizedBox(height: 16),
              CategoryChart(stats: s),
              const SizedBox(height: 16),
              MonthChart(stats: s),
              const SizedBox(height: 16),
              _MemberTable(stats: s, group: widget.group),
            ],
          ],
        ),
      ),
    );
  }
}

class StatsTotal extends ConsumerWidget {
  const StatsTotal({super.key, required this.stats, required this.label});

  final Stats stats;
  final String label;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(stats.currency);
    final text = Theme.of(context).textTheme;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: text.labelLarge),
        Text(Money.format(stats.totalSpent, info), style: text.headlineSmall),
      ],
    );
  }
}

/// Donut of spending by category with a legend.
class CategoryChart extends ConsumerWidget {
  const CategoryChart({super.key, required this.stats});

  final Stats stats;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final config = ref.watch(configValueProvider);
    final info = config.currency(stats.currency);
    final total = stats.byCategory.fold<int>(0, (a, c) => a + c.amount);
    final items = [...stats.byCategory]
      ..sort((a, b) => b.amount.compareTo(a.amount));
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('By category', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 12),
            SizedBox(
              height: 180,
              child: PieChart(
                PieChartData(
                  centerSpaceRadius: 48,
                  sectionsSpace: 2,
                  sections: [
                    for (final c in items)
                      PieChartSectionData(
                        value: c.amount.toDouble(),
                        color: categoryVisual(c.category).color,
                        radius: 34,
                        showTitle: false,
                      ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 12),
            for (final c in items)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(
                  children: [
                    CategoryIcon(category: c.category, size: 28),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        config.category(c.category)?.name ?? c.category,
                      ),
                    ),
                    Text(
                      total == 0
                          ? ''
                          : '${(c.amount * 100 / total).round()}%  ',
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.onSurfaceVariant,
                      ),
                    ),
                    Text(Money.format(c.amount, info, compact: true)),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}

/// Bars of spending per month.
class MonthChart extends ConsumerWidget {
  const MonthChart({super.key, required this.stats});

  final Stats stats;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final scheme = Theme.of(context).colorScheme;
    final info = ref.watch(configValueProvider).currency(stats.currency);
    final months = stats.byMonth.length > 12
        ? stats.byMonth.sublist(stats.byMonth.length - 12)
        : stats.byMonth;
    final unit = Money.pow10(info.exponent);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('By month', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 16),
            SizedBox(
              height: 180,
              child: BarChart(
                BarChartData(
                  gridData: const FlGridData(show: false),
                  borderData: FlBorderData(show: false),
                  barTouchData: BarTouchData(
                    touchTooltipData: BarTouchTooltipData(
                      getTooltipItem: (group, _, rod, _) => BarTooltipItem(
                        Money.format(months[group.x].amount, info),
                        TextStyle(color: scheme.onInverseSurface),
                      ),
                      getTooltipColor: (_) => scheme.inverseSurface,
                    ),
                  ),
                  titlesData: FlTitlesData(
                    leftTitles: const AxisTitles(),
                    rightTitles: const AxisTitles(),
                    topTitles: const AxisTitles(),
                    bottomTitles: AxisTitles(
                      sideTitles: SideTitles(
                        showTitles: true,
                        getTitlesWidget: (value, meta) => SideTitleWidget(
                          meta: meta,
                          child: Text(
                            Dates.monthLabel(months[value.toInt()].month),
                            style: Theme.of(context).textTheme.labelSmall,
                          ),
                        ),
                      ),
                    ),
                  ),
                  barGroups: [
                    for (var i = 0; i < months.length; i++)
                      BarChartGroupData(
                        x: i,
                        barRods: [
                          BarChartRodData(
                            toY: months[i].amount / unit,
                            color: scheme.primary,
                            width: 18,
                            borderRadius: BorderRadius.circular(6),
                          ),
                        ],
                      ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _MemberTable extends ConsumerWidget {
  const _MemberTable({required this.stats, required this.group});

  final Stats stats;
  final Group group;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(stats.currency);
    final meId = ref.watch(meProvider)?.id;
    final text = Theme.of(context).textTheme;
    if (stats.byMember.isEmpty) return const SizedBox.shrink();
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Who paid, whose share', style: text.titleMedium),
            const SizedBox(height: 8),
            for (final m in stats.byMember)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 6),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        m.userId == meId
                            ? 'You'
                            : group.member(m.userId)?.displayName ?? 'Someone',
                      ),
                    ),
                    Text('paid ${Money.format(m.paid, info, compact: true)}'),
                    const SizedBox(width: 12),
                    Text(
                      'share ${Money.format(m.owed, info, compact: true)}',
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.onSurfaceVariant,
                      ),
                    ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}
