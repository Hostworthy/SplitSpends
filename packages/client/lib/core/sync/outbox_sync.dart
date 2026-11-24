import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/core/sync/reachability.dart';

final outboxProvider = Provider<Outbox>(
  (ref) => Outbox(() => ref.read(databaseProvider.future)),
);

/// How many writes are waiting to reach the server.
final outboxCountProvider = StreamProvider<int>(
  (ref) => ref.watch(outboxProvider).count(),
);

class SyncState {
  const SyncState({
    this.syncing = false,
    this.rejected = const [],
    this.round = 0,
  });

  final bool syncing;

  /// Changes the server refused in the last replay (the server wins).
  final List<String> rejected;

  /// Increments after every replay that changed something.
  final int round;
}

/// Replays the outbox in connected mode: on start, every 20 seconds while
/// changes wait, and as soon as the server is reachable again.
final outboxSyncProvider = NotifierProvider<OutboxSync, SyncState>(
  OutboxSync.new,
);

class OutboxSync extends Notifier<SyncState> {
  Timer? _timer;

  @override
  SyncState build() {
    if (ref.watch(appModeProvider) != AppMode.connected) {
      return const SyncState();
    }
    _timer = Timer.periodic(const Duration(seconds: 20), (_) => flush());
    ref.onDispose(() => _timer?.cancel());
    ref.listen(reachabilityProvider, (was, now) {
      if (now && was == false) flush();
    });
    Future.microtask(flush);
    return const SyncState();
  }

  Future<void> flush() async {
    if (!ref.mounted || state.syncing) return;
    final outbox = ref.read(outboxProvider);
    if ((await outbox.pending()).isEmpty || !ref.mounted) return;
    state = SyncState(
      syncing: true,
      rejected: state.rejected,
      round: state.round,
    );
    final result = await outbox.flush(ref.read(apiClientProvider));
    if (!ref.mounted) return;
    final changed = result.sent > 0 || result.rejected.isNotEmpty;
    state = SyncState(
      rejected: result.rejected.isEmpty ? state.rejected : result.rejected,
      round: changed ? state.round + 1 : state.round,
    );
  }

  void dismissRejected() =>
      state = SyncState(round: state.round, syncing: state.syncing);
}
