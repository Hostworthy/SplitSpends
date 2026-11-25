import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/models/balance.dart';

/// What you owe and are owed across every group and friend.
final myBalancesProvider = FutureProvider<BalanceSummary>(
  (ref) => ref.watch(balancesRepositoryProvider).mine(),
);
