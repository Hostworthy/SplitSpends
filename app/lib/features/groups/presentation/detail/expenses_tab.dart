import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/presentation/expense_tile.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';

/// Infinite list of expenses grouped by month, with search.
class ExpenseFeedList extends ConsumerStatefulWidget {
  const ExpenseFeedList({
    super.key,
    required this.query,
    this.empty,
    this.header,
  });

  final ExpenseQuery query;
  final Widget? empty;
  final Widget? header;

  @override
  ConsumerState<ExpenseFeedList> createState() => _ExpenseFeedListState();
}

class _ExpenseFeedListState extends ConsumerState<ExpenseFeedList> {
  String _search = '';

  ExpenseQuery get _query => ExpenseQuery(
    groupId: widget.query.groupId,
    friendId: widget.query.friendId,
    q: _search.isEmpty ? null : _search,
  );

  @override
  Widget build(BuildContext context) {
    final provider = expensesFeedProvider(_query);
    final feed = ref.watch(provider);
    return NotificationListener<ScrollNotification>(
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
            final rows = <Widget>[?widget.header, _searchField()];
            if (page.items.isEmpty) {
              rows.add(
                widget.empty ??
                    const EmptyState(
                      icon: Icons.receipt_long_rounded,
                      title: 'No expenses',
                    ),
              );
            }
            String? month;
            for (final e in page.items) {
              final title = Dates.monthTitle(e.day);
              if (title != month) {
                month = title;
                rows.add(SectionHeader(title));
              }
              rows.add(ExpenseTile(expense: e));
            }
            if (page.loadingMore) {
              rows.add(
                const Padding(
                  padding: EdgeInsets.all(16),
                  child: Center(child: CircularProgressIndicator()),
                ),
              );
            }
            rows.add(const SizedBox(height: 96));
            return ListView(
              physics: const AlwaysScrollableScrollPhysics(),
              children: rows,
            );
          },
        ),
      ),
    );
  }

  Widget _searchField() => Padding(
    padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
    child: TextField(
      textInputAction: TextInputAction.search,
      onSubmitted: (v) => setState(() => _search = v.trim()),
      onChanged: (v) {
        if (v.isEmpty && _search.isNotEmpty) setState(() => _search = '');
      },
      decoration: const InputDecoration(
        isDense: true,
        prefixIcon: Icon(Icons.search_rounded),
        hintText: 'Search expenses',
      ),
    ),
  );
}

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
