import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/account/data/local_account_repository.dart';
import 'package:splitspends/features/account/data/remote_account_repository.dart';
import 'package:splitspends/features/account/domain/account_repository.dart';

final accountRepositoryProvider = Provider<AccountRepository>(
  (ref) => byMode(
    ref,
    remote: RemoteAccountRepository.new,
    local: () => LocalAccountRepository(ref.watch(localLedgerProvider)),
  ),
);

final balancesRepositoryProvider = Provider<BalancesRepository>(
  (ref) => byMode(
    ref,
    remote: RemoteBalancesRepository.new,
    local: () => LocalBalancesRepository(ref.watch(localLedgerProvider)),
  ),
);

/// API tokens exist only on a server.
final tokensRepositoryProvider = Provider<TokensRepository>(
  (ref) => byMode(ref, remote: RemoteTokensRepository.new),
);
