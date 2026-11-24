import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/balance_text.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/friends/domain/friend.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/groups/presentation/detail/expenses_tab.dart';

class FriendDetailScreen extends ConsumerWidget {
  const FriendDetailScreen({
    super.key,
    required this.userId,
    this.embedded = false,
  });

  final String userId;
  final bool embedded;

  Future<void> _remove(BuildContext context, WidgetRef ref) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Remove friend?'),
        content: const Text('Only possible when you are settled up.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Remove'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await ref.read(friendsRepositoryProvider).remove(userId);
      ref.refreshLedger();
      if (context.mounted) context.go(Routes.friends);
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final detail = ref.watch(friendDetailProvider(userId));
    final meId = ref.watch(meProvider)?.id;
    return Scaffold(
      appBar: AppBar(
        automaticallyImplyLeading: !embedded,
        title: Text(detail.value?.user.displayName ?? ''),
        actions: [
          PopupMenuButton<String>(
            onSelected: (v) {
              if (v == 'remove') _remove(context, ref);
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: 'remove', child: Text('Remove friend')),
            ],
          ),
        ],
      ),
      body: AsyncView(
        value: detail,
        onRetry: () => ref.invalidate(friendDetailProvider(userId)),
        data: (d) => ExpenseFeedList(
          query: ExpenseQuery(friendId: userId),
          header: _FriendHeader(detail: d, meId: meId),
          empty: const EmptyState(
            icon: Icons.receipt_long_rounded,
            title: 'No shared expenses yet',
          ),
        ),
      ),
    );
  }
}

class _FriendHeader extends StatelessWidget {
  const _FriendHeader({required this.detail, this.meId});

  final FriendDetail detail;
  final String? meId;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Hero(
                tag: 'friend-${detail.user.id}',
                child: UserAvatar(user: detail.user, size: 64),
              ),
              const SizedBox(width: 16),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(detail.user.displayName, style: text.titleLarge),
                    Text(
                      detail.user.isInvited
                          ? '${detail.user.email} · invited'
                          : detail.user.email,
                      style: text.bodySmall,
                    ),
                  ],
                ),
              ),
              BalanceText(
                balances: detail.balances,
                owedLabel: 'owes you',
                oweLabel: 'you owe',
              ),
            ],
          ),
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: FilledButton.icon(
                  onPressed: () => context.push(
                    Routes.settle,
                    extra: SettleArgs(
                      friendId: detail.user.id,
                      fromUserId: _owesMe ? detail.user.id : meId,
                      toUserId: _owesMe ? meId : detail.user.id,
                      amount: detail.balances.firstOrNull?.amount.abs(),
                      currency: detail.balances.firstOrNull?.currency,
                    ),
                  ),
                  icon: const Icon(Icons.handshake_rounded),
                  label: const Text('Settle up'),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: () => context.push(
                    Routes.newExpense,
                    extra: ExpenseEditorArgs(friendId: detail.user.id),
                  ),
                  icon: const Icon(Icons.add_rounded),
                  label: const Text('Add expense'),
                ),
              ),
            ],
          ),
          if (detail.groups.isNotEmpty || detail.nonGroup.isNotEmpty) ...[
            const SizedBox(height: 16),
            Card(
              child: Column(
                children: [
                  for (final g in detail.groups)
                    ListTile(
                      dense: true,
                      leading: const Icon(Icons.groups_rounded),
                      title: Text(g.groupName),
                      trailing: BalanceText(balances: g.balances, dense: true),
                      onTap: () => context.go(Routes.group(g.groupId)),
                    ),
                  if (detail.nonGroup.isNotEmpty)
                    ListTile(
                      dense: true,
                      leading: const Icon(Icons.person_rounded),
                      title: const Text('Non-group expenses'),
                      trailing: BalanceText(
                        balances: detail.nonGroup,
                        dense: true,
                      ),
                    ),
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }

  bool get _owesMe => (detail.balances.firstOrNull?.amount ?? 0) > 0;
}
