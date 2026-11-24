import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/core/sync/reachability.dart';

/// Slim status line in connected mode: offline, changes waiting to sync,
/// changes the server keeps failing on, or changes the server refused.
class SyncBar extends ConsumerWidget {
  const SyncBar({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (ref.watch(appModeProvider) != AppMode.connected) {
      return const SizedBox.shrink();
    }
    final waiting = ref.watch(outboxCountProvider).value ?? 0;
    final stuck = ref.watch(outboxStuckProvider).value ?? const [];
    final online = ref.watch(reachabilityProvider);
    final sync = ref.watch(outboxSyncProvider);
    final scheme = Theme.of(context).colorScheme;

    Widget bar(
      IconData icon,
      String text, {
      VoidCallback? onTap,
      Color? color,
    }) => Material(
      color: color ?? scheme.secondaryContainer,
      child: InkWell(
        onTap: onTap,
        child: SafeArea(
          bottom: false,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Row(
              children: [
                Icon(icon, size: 18, color: scheme.onSecondaryContainer),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    text,
                    style: Theme.of(context).textTheme.labelLarge
                        ?.copyWith(color: scheme.onSecondaryContainer),
                  ),
                ),
                if (sync.syncing)
                  const SizedBox.square(
                    dimension: 14,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
              ],
            ),
          ),
        ),
      ),
    );

    final Widget child;
    if (sync.rejected.isNotEmpty) {
      child = bar(
        Icons.sync_problem_rounded,
        '${sync.rejected.length} change${sync.rejected.length == 1 ? '' : 's'} '
        'could not be saved. Tap for details.',
        color: scheme.errorContainer,
        onTap: () => _showRejected(context, ref, sync.rejected),
      );
    } else if (stuck.isNotEmpty) {
      child = bar(
        Icons.sync_problem_rounded,
        '${stuck.length} change${stuck.length == 1 ? '' : 's'} '
        'could not be sent. Tap for details.',
        color: scheme.errorContainer,
        onTap: () => _showStuck(context),
      );
    } else if (waiting > 0) {
      child = bar(
        online ? Icons.cloud_upload_rounded : Icons.cloud_off_rounded,
        '$waiting change${waiting == 1 ? '' : 's'} waiting to sync'
        '${online ? '' : ' · offline'}',
        onTap: () => ref.read(outboxSyncProvider.notifier).retry(),
      );
    } else if (!online) {
      child = bar(Icons.cloud_off_rounded, 'Offline · showing saved data');
    } else {
      child = const SizedBox.shrink();
    }
    return AnimatedSize(
      duration: const Duration(milliseconds: 200),
      child: child,
    );
  }

  Future<void> _showStuck(BuildContext context) => showDialog<void>(
    context: context,
    builder: (context) => const _StuckDialog(),
  );

  Future<void> _showRejected(
    BuildContext context,
    WidgetRef ref,
    List<String> rejected,
  ) async {
    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Not saved'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'The server did not accept these offline changes, so its '
              'version was kept:',
            ),
            const SizedBox(height: 8),
            for (final r in rejected) Text('• $r'),
          ],
        ),
        actions: [
          FilledButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('OK'),
          ),
        ],
      ),
    );
    ref.read(outboxSyncProvider.notifier).dismissRejected();
  }
}

/// Changes the server keeps failing on, each with Discard, and Retry for
/// all of them.
class _StuckDialog extends ConsumerWidget {
  const _StuckDialog();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final stuck = ref.watch(outboxStuckProvider).value ?? const <OutboxOp>[];
    final sync = ref.read(outboxSyncProvider.notifier);
    ref.listen(outboxStuckProvider, (_, next) {
      if ((next.value ?? const []).isEmpty) Navigator.pop(context);
    });
    return AlertDialog(
      title: const Text('Not sent yet'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'The server ran into a problem with these changes. They are '
            'kept on this device; other changes still sync.',
          ),
          const SizedBox(height: 8),
          for (final op in stuck)
            ListTile(
              key: Key('stuck-${op.key}'),
              contentPadding: EdgeInsets.zero,
              title: Text(op.label),
              subtitle: op.error == null ? null : Text(op.error!),
              trailing: TextButton(
                onPressed: () => sync.discard(op.key),
                child: const Text('Discard'),
              ),
            ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Close'),
        ),
        FilledButton(
          onPressed: () {
            Navigator.pop(context);
            sync.retry();
          },
          child: const Text('Retry'),
        ),
      ],
    );
  }
}
