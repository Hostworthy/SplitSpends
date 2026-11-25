import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/activity/presentation/activity_tile.dart';
import 'package:splitspends/features/activity/presentation/providers.dart';

class ActivityScreen extends ConsumerWidget {
  const ActivityScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final provider = activityFeedProvider('');
    final feed = ref.watch(provider);
    return Scaffold(
      appBar: AppBar(title: const Text('Activity')),
      body: NotificationListener<ScrollNotification>(
        onNotification: (n) {
          if (n.metrics.extentAfter < 400) {
            ref.read(provider.notifier).loadMore();
          }
          return false;
        },
        child: RefreshIndicator(
          onRefresh: () async {
            ref.invalidate(provider);
            await ref.read(provider.future);
          },
          child: AsyncView(
            value: feed,
            onRetry: () => ref.invalidate(provider),
            data: (page) {
              if (page.items.isEmpty) {
                return ListView(
                  children: const [
                    SizedBox(height: 60),
                    EmptyState(
                      icon: Icons.bolt_rounded,
                      title: 'No activity yet',
                      message: 'Expenses, payments and comments show up here.',
                    ),
                  ],
                );
              }
              final rows = <Widget>[];
              String? day;
              for (final a in page.items) {
                final label = _dayLabel(a.time);
                if (label != day) {
                  day = label;
                  rows.add(SectionHeader(label));
                }
                rows.add(ActivityTile(activity: a));
              }
              if (page.error case final error?) {
                rows.add(
                  PageError(
                    error: error,
                    onRetry: () => ref.read(provider.notifier).retry(),
                  ),
                );
              }
              if (page.loadingMore) {
                rows.add(
                  const Padding(
                    padding: EdgeInsets.all(16),
                    child: Center(child: CircularProgressIndicator()),
                  ),
                );
              }
              rows.add(const SizedBox(height: 32));
              return ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                children: [MaxWidth(child: Column(children: rows))],
              );
            },
          ),
        ),
      ),
    );
  }

  static String _dayLabel(DateTime time) {
    final today = Dates.today();
    final day = DateTime(time.year, time.month, time.day);
    final diff = today.difference(day).inDays;
    if (diff == 0) return 'Today';
    if (diff == 1) return 'Yesterday';
    return Dates.short(day);
  }
}
