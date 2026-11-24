import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/balance_text.dart';
import 'package:splitspends/core/widgets/skeleton.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/detail/balances_tab.dart';
import 'package:splitspends/features/groups/presentation/detail/expenses_tab.dart';
import 'package:splitspends/features/groups/presentation/detail/stats_tab.dart';
import 'package:splitspends/features/groups/presentation/group_card.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

class GroupDetailScreen extends ConsumerWidget {
  const GroupDetailScreen({
    super.key,
    required this.groupId,
    this.embedded = false,
  });

  final String groupId;

  /// Shown inside the wide two-pane layout (no back button).
  final bool embedded;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final group = ref.watch(groupProvider(groupId));
    final ai = ref.watch(configValueProvider).aiEnabled;
    return DefaultTabController(
      length: 3,
      child: Scaffold(
        appBar: AppBar(
          automaticallyImplyLeading: !embedded,
          title: Text(group.value?.name ?? ''),
          actions: [
            if (ai)
              IconButton(
                tooltip: 'Scan a receipt',
                icon: const Icon(Icons.document_scanner_outlined),
                onPressed: () =>
                    context.push('${Routes.scan}?groupId=$groupId'),
              ),
            IconButton(
              tooltip: 'Settle up',
              icon: const Icon(Icons.handshake_outlined),
              onPressed: () => context.push(
                Routes.settle,
                extra: SettleArgs(groupId: groupId),
              ),
            ),
            IconButton(
              tooltip: 'Group settings',
              icon: const Icon(Icons.tune_rounded),
              onPressed: () => context.push(Routes.groupSettings(groupId)),
            ),
          ],
        ),
        body: AsyncView(
          value: group,
          onRetry: () => ref.invalidate(groupProvider(groupId)),
          loading: const _HeaderSkeleton(),
          data: (g) => Column(
            children: [
              _Header(group: g),
              const TabBar(
                tabs: [
                  Tab(text: 'Expenses'),
                  Tab(text: 'Balances'),
                  Tab(text: 'Stats'),
                ],
              ),
              Expanded(
                child: TabBarView(
                  children: [
                    GroupExpensesTab(group: g),
                    GroupBalancesTab(group: g),
                    GroupStatsTab(group: g),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.group});

  final Group group;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 12),
      child: Row(
        children: [
          GroupBadge(group: group, size: 56),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  group.name,
                  style: text.titleLarge,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                const SizedBox(height: 6),
                AvatarStack(users: group.members, size: 24),
              ],
            ),
          ),
          BalanceText(balances: group.balances),
        ],
      ),
    );
  }
}

class _HeaderSkeleton extends StatelessWidget {
  const _HeaderSkeleton();

  @override
  Widget build(BuildContext context) => const Column(
    children: [
      Padding(
        padding: EdgeInsets.all(16),
        child: Row(
          children: [
            Skeleton(height: 56, width: 56, radius: 16),
            SizedBox(width: 14),
            Expanded(child: Skeleton(height: 20)),
          ],
        ),
      ),
      Expanded(child: SkeletonList()),
    ],
  );
}
