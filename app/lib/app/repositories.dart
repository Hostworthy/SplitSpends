import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/account/data/remote_account_repository.dart';
import 'package:splitspends/features/account/domain/account_repository.dart';
import 'package:splitspends/features/activity/data/remote_activity_repository.dart';
import 'package:splitspends/features/activity/domain/activity_repository.dart';
import 'package:splitspends/features/auth/data/remote_auth_repository.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';
import 'package:splitspends/features/expenses/data/remote_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/friends/data/remote_friends_repository.dart';
import 'package:splitspends/features/friends/domain/friends_repository.dart';
import 'package:splitspends/features/groups/data/remote_groups_repository.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/receipts/data/remote_receipts_repository.dart';
import 'package:splitspends/features/receipts/domain/receipts_repository.dart';

// Composition root: the only place that chooses an implementation per mode.
// Screens and providers depend on the domain interfaces only.

/// Picks the implementation for the current mode.
T _select<T>(
  Ref ref, {
  required T Function(ApiClient api) remote,
  T Function()? local,
}) {
  switch (ref.watch(appModeProvider)) {
    case AppMode.connected:
      return remote(ref.watch(apiClientProvider));
    case AppMode.local:
    case AppMode.unset:
      if (local != null) return local();
      throw const ApiError(
        status: 501,
        code: 'LOCAL_UNAVAILABLE',
        message: 'This works only when connected to a server.',
      );
  }
}

// TODO(phase 2): pass `local:` factories backed by the on-device store.

final groupsRepositoryProvider = Provider<GroupsRepository>(
  (ref) => _select(ref, remote: RemoteGroupsRepository.new),
);

final expensesRepositoryProvider = Provider<ExpensesRepository>(
  (ref) => _select(ref, remote: RemoteExpensesRepository.new),
);

final friendsRepositoryProvider = Provider<FriendsRepository>(
  (ref) => _select(ref, remote: RemoteFriendsRepository.new),
);

final activityRepositoryProvider = Provider<ActivityRepository>(
  (ref) => _select(ref, remote: RemoteActivityRepository.new),
);

final accountRepositoryProvider = Provider<AccountRepository>(
  (ref) => _select(ref, remote: RemoteAccountRepository.new),
);

final balancesRepositoryProvider = Provider<BalancesRepository>(
  (ref) => _select(ref, remote: RemoteBalancesRepository.new),
);

/// API tokens exist only on a server.
final tokensRepositoryProvider = Provider<TokensRepository>(
  (ref) => _select(ref, remote: RemoteTokensRepository.new),
);

/// Receipts and AI need a server (AI is a server-side feature).
final receiptsRepositoryProvider = Provider<ReceiptsRepository>(
  (ref) => _select(ref, remote: RemoteReceiptsRepository.new),
);

/// Login always talks to the server being connected to.
final authRepositoryProvider = Provider<AuthRepository>(
  (ref) => RemoteAuthRepository(ref.read(httpClientProvider)),
);
