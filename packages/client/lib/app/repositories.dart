import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/features/expenses/data/offline_expenses_repository.dart';
import 'package:splitspends/features/groups/data/offline_groups_repository.dart';
import 'package:splitspends/features/account/data/local_account_repository.dart';
import 'package:splitspends/features/activity/data/local_activity_repository.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/friends/data/local_friends_repository.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/receipts/data/local_receipts_repository.dart';
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
import 'package:splitspends/features/invites/data/remote_invites_repository.dart';
import 'package:splitspends/features/invites/domain/invite.dart';
import 'package:splitspends/features/receipts/data/remote_receipts_repository.dart';
import 'package:splitspends/features/receipts/domain/receipts_repository.dart';
import 'package:splitspends/features/reminders/data/remote_reminders_repository.dart';
import 'package:splitspends/features/reminders/domain/reminder.dart';
import 'package:splitspends/features/sync/data/backup_service.dart';
import 'package:splitspends/features/sync/data/import_service.dart';

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
      throw const ApiError.needsServer(
        'This works only when connected to a server.',
      );
  }
}

/// The on-device ledger used by every local implementation.
final localLedgerProvider = Provider<LocalLedger>(
  (ref) => LocalLedger(() => ref.read(databaseProvider.future)),
);

final groupsRepositoryProvider = Provider<GroupsRepository>(
  (ref) => _select(
    ref,
    remote: (api) => OfflineGroupsRepository(
      RemoteGroupsRepository(api),
      ref.watch(outboxProvider),
      me: () => ref.read(meProvider),
      onQueued: () {},
    ),
    local: () => LocalGroupsRepository(ref.watch(localLedgerProvider)),
  ),
);

final expensesRepositoryProvider = Provider<ExpensesRepository>(
  (ref) => _select(
    ref,
    remote: (api) => OfflineExpensesRepository(
      RemoteExpensesRepository(api),
      ref.watch(outboxProvider),
      meId: () => ref.read(meProvider)?.id ?? '',
      onQueued: () {},
    ),
    local: () => LocalExpensesRepository(ref.watch(localLedgerProvider)),
  ),
);

final friendsRepositoryProvider = Provider<FriendsRepository>(
  (ref) => _select(
    ref,
    remote: RemoteFriendsRepository.new,
    local: () => LocalFriendsRepository(ref.watch(localLedgerProvider)),
  ),
);

final activityRepositoryProvider = Provider<ActivityRepository>(
  (ref) => _select(
    ref,
    remote: RemoteActivityRepository.new,
    local: () => LocalActivityRepository(ref.watch(localLedgerProvider)),
  ),
);

final accountRepositoryProvider = Provider<AccountRepository>(
  (ref) => _select(
    ref,
    remote: RemoteAccountRepository.new,
    local: () => LocalAccountRepository(ref.watch(localLedgerProvider)),
  ),
);

final balancesRepositoryProvider = Provider<BalancesRepository>(
  (ref) => _select(
    ref,
    remote: RemoteBalancesRepository.new,
    local: () => LocalBalancesRepository(ref.watch(localLedgerProvider)),
  ),
);

/// API tokens exist only on a server.
final tokensRepositoryProvider = Provider<TokensRepository>(
  (ref) => _select(ref, remote: RemoteTokensRepository.new),
);

/// Receipts and AI need a server (AI is a server-side feature).
final receiptsRepositoryProvider = Provider<ReceiptsRepository>(
  (ref) => _select(
    ref,
    remote: RemoteReceiptsRepository.new,
    local: () => const LocalReceiptsRepository(),
  ),
);

/// Emailed reminders need a server; on this device people share a text.
final remindersRepositoryProvider = Provider<RemindersRepository>(
  (ref) => _select(ref, remote: RemoteRemindersRepository.new),
);

/// Invite links exist only on a server.
final invitesRepositoryProvider = Provider<InvitesRepository>(
  (ref) => _select(ref, remote: RemoteInvitesRepository.new),
);

/// Link previews are public, so they work before signing in.
final invitePreviewsProvider = Provider<InvitePreviews>(
  (ref) => RemoteInvitePreviews(ref.read(httpClientProvider)),
);

/// Login always talks to the server being connected to.
final authRepositoryProvider = Provider<AuthRepository>(
  (ref) => RemoteAuthRepository(ref.read(httpClientProvider)),
);

/// Uploads on-device data to an account.
final importServiceProvider = Provider<ImportService>(
  (ref) => ImportService(
    ref.watch(localLedgerProvider),
    ref.read(httpClientProvider),
  ),
);

final backupServiceProvider = Provider<BackupService>(
  (ref) => BackupService(ref.watch(localLedgerProvider)),
);

/// Creates due recurring expenses on start in on-device mode (the server
/// does this itself in connected mode). Yields how many were created.
final localMaintenanceProvider = FutureProvider<int>((ref) async {
  if (ref.watch(appModeProvider) != AppMode.local) return 0;
  return ref.watch(localLedgerProvider).runRecurring();
});
