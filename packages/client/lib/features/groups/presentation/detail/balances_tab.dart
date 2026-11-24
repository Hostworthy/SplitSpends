import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/balance_text.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

class GroupBalancesTab extends ConsumerWidget {
  const GroupBalancesTab({super.key, required this.group});

  final Group group;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final balances = ref.watch(groupBalancesProvider(group.id));
    final meId = ref.watch(meProvider)?.id;
    final config = ref.watch(configValueProvider);
    final text = Theme.of(context).textTheme;
    final colors = BalanceColors.of(context);
    String name(String id) =>
        id == meId ? 'You' : (group.member(id)?.firstName ?? 'Someone');

    return RefreshIndicator(
      onRefresh: () async {
        ref.invalidate(groupBalancesProvider(group.id));
        await ref.read(groupBalancesProvider(group.id).future);
      },
      child: AsyncView(
        value: balances,
        onRetry: () => ref.invalidate(groupBalancesProvider(group.id)),
        data: (data) => ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.only(bottom: 96),
          children: [
            SectionHeader(
              group.simplifyDebts ? 'Settle up (simplified)' : 'Settle up',
            ),
            if (data.debts.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 20, vertical: 8),
                child: Text('Everyone is settled up. 🎉'),
              ),
            for (final debt in data.debts)
              ListTile(
                leading: _DebtAvatars(
                  from: group.member(debt.from),
                  to: group.member(debt.to),
                ),
                title: Text.rich(
                  TextSpan(
                    children: [
                      TextSpan(
                        text: name(debt.from),
                        style: const TextStyle(fontWeight: FontWeight.w700),
                      ),
                      TextSpan(text: debt.from == meId ? ' owe ' : ' owes '),
                      TextSpan(
                        text: name(debt.to),
                        style: const TextStyle(fontWeight: FontWeight.w700),
                      ),
                    ],
                  ),
                ),
                subtitle: Text(
                  Money.format(debt.amount, config.currency(debt.currency)),
                  style: text.titleSmall?.copyWith(
                    color: debt.from == meId
                        ? colors.owe
                        : debt.to == meId
                        ? colors.owed
                        : null,
                  ),
                ),
                trailing: FilledButton.tonal(
                  onPressed: () => context.push(
                    Routes.settle,
                    extra: SettleArgs(
                      groupId: group.id,
                      fromUserId: debt.from,
                      toUserId: debt.to,
                      amount: debt.amount,
                      currency: debt.currency,
                    ),
                  ),
                  child: const Text('Settle'),
                ),
              ),
            const SectionHeader('Members'),
            for (final member in group.members)
              ListTile(
                leading: UserAvatar(user: member),
                title: Text(member.id == meId ? 'You' : member.displayName),
                subtitle: member.isInvited ? const Text('Invited') : null,
                trailing: BalanceText(
                  balances: data.members
                      .firstWhere(
                        (m) => m.userId == member.id,
                        orElse: () => MemberBalance(
                          userId: member.id,
                          balances: const [],
                        ),
                      )
                      .balances,
                  owedLabel: 'gets back',
                  oweLabel: 'owes',
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _DebtAvatars extends StatelessWidget {
  const _DebtAvatars({this.from, this.to});

  final User? from;
  final User? to;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 74,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          if (from != null) UserAvatar(user: from!, size: 28),
          const Icon(Icons.arrow_forward_rounded, size: 14),
          if (to != null) UserAvatar(user: to!, size: 28),
        ],
      ),
    );
  }
}
