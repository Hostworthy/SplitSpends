import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/core/sync/reachability.dart';

/// Slim status line in connected mode: offline, changes waiting to sync,
/// or changes the server refused.
class SyncBar extends ConsumerWidget {
  const SyncBar({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (ref.watch(appModeProvider) != AppMode.connected) {
      return const SizedBox.shrink();
    }
    final waiting = ref.watch(outboxCountProvider).value ?? 0;
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
    } else if (waiting > 0) {
      child = bar(
        online ? Icons.cloud_upload_rounded : Icons.cloud_off_rounded,
        '$waiting change${waiting == 1 ? '' : 's'} waiting to sync'
        '${online ? '' : ' · offline'}',
        onTap: () => ref.read(outboxSyncProvider.notifier).flush(),
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
