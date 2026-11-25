import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/routes.dart';
import 'package:splitspends/features/expenses/presentation/route_args.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/presentation/expense_feed_list.dart';
import 'package:splitspends/features/groups/domain/group.dart';

class GroupExpensesTab extends StatelessWidget {
  const GroupExpensesTab({super.key, required this.group});

  final Group group;

  @override
  Widget build(BuildContext context) {
    return ExpenseFeedList(
      query: ExpenseQuery(groupId: group.id),
      empty: EmptyState(
        icon: Icons.receipt_long_rounded,
        title: 'No expenses yet',
        message: 'Add the first expense for ${group.name}.',
        action: FilledButton.icon(
          onPressed: () => context.push(
            Routes.newExpense,
            extra: ExpenseEditorArgs(groupId: group.id),
          ),
          icon: const Icon(Icons.add_rounded),
          label: const Text('Add expense'),
        ),
      ),
    );
  }
}
