import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';

/// Picks the repository implementation for the current mode. Each feature
/// declares its repository providers with this in its data/providers.dart,
/// so screens and providers depend on the domain interfaces only.
T byMode<T>(
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

/// Creates due recurring expenses on start in on-device mode (the server
/// does this itself in connected mode). Yields how many were created.
final localMaintenanceProvider = FutureProvider<int>((ref) async {
  if (ref.watch(appModeProvider) != AppMode.local) return 0;
  return ref.watch(localLedgerProvider).runRecurring();
});
