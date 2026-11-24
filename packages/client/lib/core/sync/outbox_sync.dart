import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/core/sync/reachability.dart';

/// The signed-in account's outbox; a new one whenever the account changes.
final outboxProvider = Provider<Outbox>(
  (ref) => Outbox(
    () => ref.read(databaseProvider.future),
    owner: ref.watch(sessionProvider.select((s) => s.value?.owner)),
  ),
);

/// How many of the account's writes are waiting to reach the server.
final outboxCountProvider = StreamProvider<int>(
  (ref) => ref.watch(outboxProvider).count(),
);

/// The account's writes the server keeps failing on.
final outboxStuckProvider = StreamProvider<List<OutboxOp>>(
  (ref) => ref.watch(outboxProvider).stuck(),
);

/// Writes on this device that belong to another account or server; they
/// wait for that account to sign in again.
final outboxOthersProvider = StreamProvider<int>(
  (ref) => ref.watch(outboxProvider).others(),
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

  /// Set while the server asked to be left alone (Retry-After).
  Timer? _pause;

  /// Set synchronously for the whole pass, so overlapping triggers never
  /// send the same writes twice.
  bool _running = false;

  static const _longestPause = Duration(minutes: 15);

  @override
  SyncState build() {
    if (ref.watch(appModeProvider) != AppMode.connected) {
      return const SyncState();
    }
    // Rebuilds, and so replays, when another account signs in.
    ref.watch(outboxProvider);
    _timer = Timer.periodic(const Duration(seconds: 20), (_) => flush());
    ref.onDispose(() {
      _timer?.cancel();
      _pause?.cancel();
    });
    ref.listen(reachabilityProvider, (was, now) {
      if (now && was == false) flush();
    });
    Future.microtask(flush);
    return const SyncState();
  }

  /// Replays the outbox unless a pass is running or the server asked for a
  /// pause.
  Future<void> flush() async {
    if (!ref.mounted || _running || (_pause?.isActive ?? false)) return;
    _running = true;
    try {
      await _pass();
    } finally {
      _running = false;
      if (ref.mounted && state.syncing) {
        state = SyncState(rejected: state.rejected, round: state.round);
      }
    }
  }

  /// Gives stuck writes a fresh start and replays now.
  Future<void> retry() async {
    _pause?.cancel();
    await ref.read(outboxProvider).unstick();
    await flush();
  }

  /// Drops one stuck write for good.
  Future<void> discard(int key) => ref.read(outboxProvider).discard(key);

  Future<void> _pass() async {
    final outbox = ref.read(outboxProvider);
    if ((await outbox.pending()).isEmpty || !ref.mounted) return;
    state = SyncState(
      syncing: true,
      rejected: state.rejected,
      round: state.round,
    );
    final result = await outbox.flush(ref.read(apiClientProvider));
    if (!ref.mounted) return;
    final wait = result.retryAfter;
    if (wait != null && wait > Duration.zero) {
      _pause?.cancel();
      // A misconfigured server must not stop syncing for days.
      _pause = Timer(wait < _longestPause ? wait : _longestPause, () {});
    }
    final changed = result.sent > 0 || result.rejected.isNotEmpty;
    state = SyncState(
      rejected: result.rejected.isEmpty ? state.rejected : result.rejected,
      round: changed ? state.round + 1 : state.round,
    );
  }

  void dismissRejected() =>
      state = SyncState(round: state.round, syncing: state.syncing);
}
