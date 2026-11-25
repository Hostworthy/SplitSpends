import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/dashboard/presentation/providers.dart';
import 'package:splitspends/features/activity/presentation/activity_tile.dart';
import 'package:splitspends/features/activity/presentation/providers.dart';
import 'package:splitspends/features/dashboard/presentation/summary_card.dart';
import 'package:splitspends/features/friends/presentation/add_friend_dialog.dart';
import 'package:splitspends/features/groups/presentation/group_card.dart';
import 'package:splitspends/features/groups/presentation/group_form_sheet.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';
import 'package:splitspends/features/receipts/presentation/ai_quick_add_sheet.dart';

class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key});

  String _greeting() {
    final hour = DateTime.now().hour;
    if (hour < 12) return 'Good morning';
    if (hour < 18) return 'Good afternoon';
    return 'Good evening';
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final me = ref.watch(meProvider);
    final balances = ref.watch(myBalancesProvider);
    final groups = ref.watch(groupsProvider);
    final activity = ref.watch(activityFeedProvider(''));
    final ai = ref.watch(configValueProvider).aiEnabled;
    final text = Theme.of(context).textTheme;

    return Scaffold(
      body: RefreshIndicator(
        onRefresh: () async {
          ref.refreshLedger();
          await ref.read(myBalancesProvider.future);
        },
        child: CustomScrollView(
          slivers: [
            SliverToBoxAdapter(
              child: MaxWidth(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    SafeArea(
                      bottom: false,
                      child: Padding(
                        padding: const EdgeInsets.fromLTRB(20, 24, 16, 16),
                        child: Row(
                          children: [
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    _greeting(),
                                    style: text.titleMedium?.copyWith(
                                      color: Theme.of(context)
                                          .colorScheme
                                          .onSurfaceVariant,
                                    ),
                                  ),
                                  Text(
                                    me?.firstName ?? 'Split Spends',
                                    style: text.headlineMedium,
                                  ),
                                ],
                              ),
                            ),
                            if (me != null)
                              GestureDetector(
                                onTap: () => context.go(Routes.account),
                                child: UserAvatar(user: me, size: 44),
                              ),
                          ],
                        ),
                      ),
                    ),
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 16),
                      child: AsyncView(
                        value: balances,
                        onRetry: () => ref.invalidate(myBalancesProvider),
                        loading: const SummaryCardSkeleton(),
                        data: (summary) => SummaryCard(summary: summary),
                      ),
                    ),
                    const SizedBox(height: 16),
                    _QuickActions(ai: ai),
                    SectionHeader(
                      'Groups',
                      trailing: TextButton(
                        onPressed: () => context.go(Routes.groups),
                        child: const Text('See all'),
                      ),
                    ),
                    SizedBox(
                      height: 132,
                      child: switch (groups) {
                        AsyncValue(:final value?) when value.isEmpty => Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 16),
                          child: NewGroupCard(
                            onTap: () => showNewGroupSheet(context),
                          ),
                        ),
                        AsyncValue(:final value?) => ListView.separated(
                          scrollDirection: Axis.horizontal,
                          padding: const EdgeInsets.symmetric(horizontal: 16),
                          itemCount: value.length + 1,
                          separatorBuilder: (_, _) => const SizedBox(width: 12),
                          itemBuilder: (context, i) => i == value.length
                              ? NewGroupCard(
                                  onTap: () => showNewGroupSheet(context),
                                )
                              : GroupCard(group: value[i]),
                        ),
                        _ => const SizedBox.shrink(),
                      },
                    ),
                    SectionHeader(
                      'Recent activity',
                      trailing: TextButton(
                        onPressed: () => context.go(Routes.activity),
                        child: const Text('See all'),
                      ),
                    ),
                    AsyncView(
                      value: activity,
                      onRetry: () => ref.invalidate(activityFeedProvider),
                      data: (page) => page.items.isEmpty
                          ? Padding(
                              padding: const EdgeInsets.all(24),
                              child: Text(
                                'Nothing yet. Add your first expense!',
                                style: text.bodyMedium,
                                textAlign: TextAlign.center,
                              ),
                            )
                          : Column(
                              children: [
                                for (final a in page.items.take(6))
                                  ActivityTile(activity: a),
                              ],
                            ),
                    ),
                    const SizedBox(height: 96),
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

class _QuickActions extends StatelessWidget {
  const _QuickActions({required this.ai});

  final bool ai;

  @override
  Widget build(BuildContext context) {
    final actions = <(IconData, String, VoidCallback)>[
      (
        Icons.add_card_rounded,
        'Add expense',
        () => context.push(Routes.newExpense),
      ),
      (Icons.handshake_rounded, 'Settle up', () => context.push(Routes.settle)),
      if (ai)
        (
          Icons.document_scanner_rounded,
          'Scan receipt',
          () => context.push(Routes.scan),
        ),
      if (ai)
        (
          Icons.auto_awesome_rounded,
          'Ask AI',
          () => showAiQuickAddSheet(context),
        ),
      (Icons.group_add_rounded, 'New group', () => showNewGroupSheet(context)),
      (
        Icons.person_add_alt_1_rounded,
        'Add friend',
        () => showAddFriendDialog(context),
      ),
    ];
    return SizedBox(
      height: 44,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        itemCount: actions.length,
        separatorBuilder: (_, _) => const SizedBox(width: 8),
        itemBuilder: (context, i) {
          final (icon, label, onTap) = actions[i];
          return ActionChip(
            avatar: Icon(icon, size: 18),
            label: Text(label),
            onPressed: onTap,
          );
        },
      ),
    );
  }
}
