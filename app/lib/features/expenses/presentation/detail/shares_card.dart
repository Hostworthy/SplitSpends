import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/friends/presentation/people.dart';

/// Who paid what and who owes what, plus items for itemized expenses.
class SharesCard extends ConsumerWidget {
  const SharesCard({super.key, required this.expense});

  final Expense expense;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(expense.currency);
    final people = ref.watch(peopleProvider);
    final meId = ref.watch(meProvider)?.id;
    final colors = BalanceColors.of(context);
    final text = Theme.of(context).textTheme;
    String money(int v) => Money.format(v, info);

    return Card(
      margin: const EdgeInsets.fromLTRB(16, 12, 16, 0),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 4),
              child: Text(
                expense.isPayment
                    ? 'Payment'
                    : 'Split ${expense.splitMethod.label.toLowerCase()}',
                style: text.labelLarge,
              ),
            ),
            for (final s in expense.shares)
              ListTile(
                leading: people[s.userId] == null
                    ? const CircleAvatar(child: Icon(Icons.person))
                    : UserAvatar(user: people[s.userId]!),
                title: Text(people.nameOf(s.userId, meId: meId, full: true)),
                subtitle: Text(
                  [
                    if (s.paid > 0) 'paid ${money(s.paid)}',
                    if (s.owed > 0)
                      expense.isPayment
                          ? 'received ${money(s.owed)}'
                          : (s.userId == meId
                                ? 'owe ${money(s.owed)}'
                                : 'owes ${money(s.owed)}'),
                  ].join(' · '),
                ),
                trailing: s.net == 0
                    ? null
                    : Text(
                        '${s.net > 0 ? '+' : '−'}${Money.format(s.net, info, absolute: true)}',
                        style: text.titleSmall?.copyWith(
                          color: colors.forAmount(s.net),
                          fontWeight: FontWeight.w700,
                        ),
                      ),
              ),
            if (expense.items.isNotEmpty) ...[
              const Divider(),
              for (final item in expense.items)
                ListTile(
                  dense: true,
                  title: Text(item.name),
                  subtitle: Text(
                    item.userIds
                        .map((id) => people.nameOf(id, meId: meId))
                        .join(', '),
                  ),
                  trailing: Text(money(item.amount)),
                ),
              if (expense.tax != 0)
                ListTile(
                  dense: true,
                  title: const Text('Tax'),
                  trailing: Text(money(expense.tax)),
                ),
              if (expense.tip != 0)
                ListTile(
                  dense: true,
                  title: const Text('Tip'),
                  trailing: Text(money(expense.tip)),
                ),
            ],
          ],
        ),
      ),
    );
  }
}
