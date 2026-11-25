import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/features/account/data/providers.dart';
import 'package:splitspends/core/models/balance.dart';

/// What you owe and are owed across every group and friend.
final myBalancesProvider = FutureProvider<BalanceSummary>((ref) {
  ref.watch(ledgerVersionProvider);
  return ref.watch(balancesRepositoryProvider).mine();
});
