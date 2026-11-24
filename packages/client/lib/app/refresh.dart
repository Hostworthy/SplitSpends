import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show ProviderOrFamily;

import 'package:splitspends/features/account/presentation/providers.dart';
import 'package:splitspends/features/activity/presentation/providers.dart';
import 'package:splitspends/features/categories/presentation/providers.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

/// Invalidates everything that depends on the ledger after a change
/// (expense, payment, membership). Cheap: only watched providers refetch.
extension LedgerRefresh on WidgetRef {
  void refreshLedger() => _refresh(invalidate);
}

/// Same as [LedgerRefresh.refreshLedger] from outside a widget.
void refreshLedgerIn(ProviderContainer container) =>
    _refresh(container.invalidate);

void _refresh(void Function(ProviderOrFamily) invalidate) {
  invalidate(myBalancesProvider);
  invalidate(groupsProvider);
  invalidate(archivedGroupsProvider);
  invalidate(groupProvider);
  invalidate(groupBalancesProvider);
  invalidate(statsProvider);
  invalidate(friendsProvider);
  invalidate(friendDetailProvider);
  invalidate(expensesFeedProvider);
  invalidate(expenseProvider);
  invalidate(activityFeedProvider);
  invalidate(expenseHistoryProvider);
  invalidate(categoriesProvider);
}
