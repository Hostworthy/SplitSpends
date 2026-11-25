import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/paging/paged.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/skeleton.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
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
  final _searchText = TextEditingController();
  String _search = '';

  ExpenseQuery get _query => ExpenseQuery(
    groupId: widget.query.groupId,
    friendId: widget.query.friendId,
    category: widget.query.category,
    q: _search.isEmpty ? null : _search,
  );

  @override
  void dispose() {
    _searchText.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final provider = expensesFeedProvider(_query);
    final feed = ref.watch(provider);
    // The search box stays above the list while a search loads or fails,
    // so the person can always see and clear what filters it.
    final rows = <Widget>[?widget.header, _searchField()];
    switch (feed) {
      case AsyncValue(:final value?):
        rows.addAll(
          _rows(value, retry: () => ref.read(provider.notifier).retry()),
        );
      case AsyncValue(:final error?):
        rows.add(
          ErrorView(error: error, onRetry: () => ref.invalidate(provider)),
        );
      default:
        rows.add(const SkeletonList());
    }
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
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          children: rows,
        ),
      ),
    );
  }

  List<Widget> _rows(Paged<Expense> page, {required VoidCallback retry}) {
    final rows = <Widget>[];
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
    if (page.error case final error?) {
      rows.add(PageError(error: error, onRetry: retry));
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
    return rows;
  }

  void _clear() {
    _searchText.clear();
    setState(() => _search = '');
  }

  Widget _searchField() => Padding(
    padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
    child: TextField(
      key: const Key('expense-search'),
      controller: _searchText,
      textInputAction: TextInputAction.search,
      onSubmitted: (v) => setState(() => _search = v.trim()),
      onChanged: (v) {
        if (v.isEmpty && _search.isNotEmpty) setState(() => _search = '');
      },
      decoration: InputDecoration(
        isDense: true,
        prefixIcon: const Icon(Icons.search_rounded),
        hintText: 'Search expenses',
        suffixIcon: _search.isEmpty
            ? null
            : IconButton(
                tooltip: 'Clear search',
                icon: const Icon(Icons.close_rounded),
                onPressed: _clear,
              ),
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
