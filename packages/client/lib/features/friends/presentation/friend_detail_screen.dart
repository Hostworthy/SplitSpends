import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/core/routes.dart';
import 'package:splitspends/features/expenses/presentation/route_args.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/balance_text.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/presentation/expense_feed_list.dart';
import 'package:splitspends/features/friends/data/providers.dart';
import 'package:splitspends/features/friends/domain/friend.dart';
import 'package:splitspends/features/friends/presentation/add_friend_dialog.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/reminders/presentation/remind_button.dart';

class FriendDetailScreen extends ConsumerWidget {
  const FriendDetailScreen({
    super.key,
    required this.userId,
    this.embedded = false,
  });

  final String userId;
  final bool embedded;

  Future<void> _remove(BuildContext context) async {
    final ok = await confirm(
      context,
      title: 'Remove friend?',
      message: 'Only possible when you are settled up.',
      action: 'Remove',
    );
    if (!ok || !context.mounted) return;
    final removed = await changeLedger(
      context,
      (container) => container.read(friendsRepositoryProvider).remove(userId),
    );
    if (removed && context.mounted) context.go(Routes.friends);
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
              if (v == 'remove') _remove(context);
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
          header: _FriendHeader(
            detail: d,
            meId: meId,
            local: ref.watch(appModeProvider) == AppMode.local,
          ),
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
  const _FriendHeader({required this.detail, this.meId, this.local = false});

  final FriendDetail detail;
  final String? meId;
  final bool local;

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
                    Text(detail.user.statusLine, style: text.bodySmall),
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
          if (_owed.isNotEmpty) ...[
            const SizedBox(height: 8),
            RemindButton(user: detail.user, owed: _owed, compact: false),
          ],
          if (detail.user.email == null) ...[
            const SizedBox(height: 12),
            Card(
              child: ListTile(
                leading: const Icon(Icons.forward_to_inbox_rounded),
                title: Text('Add ${detail.user.firstName}’s email'),
                subtitle: Text(
                  local
                      ? 'Optional. Helps match them if you connect to a server later.'
                      : 'Invite them so they can see and add expenses too.',
                ),
                onTap: () => showInviteDialog(
                  context,
                  detail.user.id,
                  detail.user.firstName,
                ),
              ),
            ),
          ],
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

  /// What the friend owes you overall, per currency.
  List<Balance> get _owed => [
    for (final b in detail.balances)
      if (b.amount > 0) b,
  ];
}
