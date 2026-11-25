import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/paging/paged.dart';
import 'package:splitspends/features/activity/domain/activity.dart';
import 'package:splitspends/features/activity/data/providers.dart';
import 'package:splitspends/features/expenses/data/providers.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

final expensesFeedProvider =
    AsyncNotifierProvider.family<ExpensesFeed, Paged<Expense>, ExpenseQuery>(
      ExpensesFeed.new,
    );

class ExpensesFeed extends PagedNotifier<Expense> {
  ExpensesFeed(this.query);

  final ExpenseQuery query;

  @override
  Future<PageResult<Expense>> fetch(int offset, int limit) => ref
      .read(expensesRepositoryProvider)
      .list(query, limit: limit, offset: offset);

  @override
  Object key(Expense item) => item.id;

  @override
  Future<Paged<Expense>> build() {
    ref
      ..watch(ledgerVersionProvider)
      ..watch(expensesRepositoryProvider);
    return super.build();
  }
}

final expenseProvider = FutureProvider.family<Expense, String>((ref, id) {
  ref.watch(ledgerVersionProvider);
  return ref.watch(expensesRepositoryProvider).get(id);
});

final commentsProvider = FutureProvider.family<List<Comment>, String>(
  (ref, id) => ref.watch(expensesRepositoryProvider).comments(id),
);

/// Changes to one expense, newest first.
final expenseHistoryProvider = FutureProvider.family<List<Activity>, String>((
  ref,
  id,
) async {
  ref.watch(ledgerVersionProvider);
  return (await ref.watch(activityRepositoryProvider).list(expenseId: id))
      .items;
});
