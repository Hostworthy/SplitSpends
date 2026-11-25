import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/features/categories/presentation/category_badge.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/friends/presentation/people.dart';

/// A row in an expense list: date, icon, description, who paid, your share.
class ExpenseTile extends ConsumerWidget {
  const ExpenseTile({super.key, required this.expense, this.groupName});

  final Expense expense;
  final String? groupName;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final people = ref.watch(peopleProvider);
    final meId = ref.watch(meProvider)?.id;
    final info = ref.watch(configValueProvider).currency(expense.currency);
    final colors = BalanceColors.of(context);
    final theme = Theme.of(context);
    final muted = theme.colorScheme.onSurfaceVariant;
    final day = expense.day;

    String subtitle;
    if (expense.isPayment) {
      final from = expense.payers.firstOrNull?.userId ?? '';
      final to = expense.shares
          .firstWhere(
            (s) => s.owed > 0,
            orElse: () => const Share(userId: '', paid: 0, owed: 0),
          )
          .userId;
      subtitle =
          '${people.nameOf(from, meId: meId)} paid ${to == meId ? 'you' : people.nameOf(to, meId: meId)}';
    } else {
      final payers = expense.payers;
      final who = payers.length == 1
          ? people.nameOf(payers.first.userId, meId: meId)
          : '${payers.length} people';
      subtitle =
          '$who paid ${Money.format(expense.total, info)}'
          '${groupName == null ? '' : ' · $groupName'}';
    }

    if (expense.pending) subtitle = 'Waiting to sync · $subtitle';

    final mine = expense.myBalance;
    final label = mine > 0
        ? (expense.isPayment ? 'you paid' : 'you lent')
        : mine < 0
        ? (expense.isPayment ? 'you received' : 'you borrowed')
        : (expense.shareOf(meId ?? '') == null ? 'not involved' : 'settled');

    return Opacity(
      opacity: expense.deleted ? 0.5 : 1,
      child: ListTile(
        onTap: () => context.push(Routes.expense(expense.id)),
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 2),
        leading: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            SizedBox(
              width: 34,
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Text(
                    DateFormat.MMM().format(day).toUpperCase(),
                    style: theme.textTheme.labelSmall?.copyWith(color: muted),
                  ),
                  Text(
                    '${day.day}',
                    style: theme.textTheme.titleMedium?.copyWith(height: 1.1),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Hero(
              tag: 'expense-icon-${expense.id}',
              child: CategoryBadge(
                category: expense.category,
                payment: expense.isPayment,
              ),
            ),
          ],
        ),
        title: Text(
          expense.isPayment ? 'Payment' : expense.description,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(
            decoration: expense.deleted ? TextDecoration.lineThrough : null,
          ),
        ),
        subtitle: Text(
          subtitle,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(color: muted),
        ),
        trailing: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            Text(
              label,
              style: theme.textTheme.labelSmall?.copyWith(
                color: colors.forAmount(mine),
              ),
            ),
            if (mine != 0)
              Text(
                Money.format(mine, info, absolute: true),
                style: theme.textTheme.titleSmall?.copyWith(
                  color: colors.forAmount(mine),
                  fontWeight: FontWeight.w700,
                ),
              ),
          ],
        ),
      ),
    );
  }
}
