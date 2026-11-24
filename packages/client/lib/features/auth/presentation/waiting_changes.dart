import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/sync/outbox_sync.dart';

/// Tells people signing in that changes from an earlier session are still
/// on this device (its session ended before they synced), and lets them
/// discard those changes.
class WaitingChanges extends ConsumerWidget {
  const WaitingChanges({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final waiting = ref.watch(outboxOthersProvider).value ?? 0;
    if (waiting == 0) return const SizedBox.shrink();
    final scheme = Theme.of(context).colorScheme;
    final changes = '$waiting change${waiting == 1 ? '' : 's'}';
    return Card(
      key: const Key('waiting-changes'),
      color: scheme.secondaryContainer,
      margin: const EdgeInsets.only(bottom: 20),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 8, 4),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              '$changes made on this device ${waiting == 1 ? 'has' : 'have'} '
              'not synced yet. Sign in again to the same account and server '
              'to send ${waiting == 1 ? 'it' : 'them'}.',
              style: TextStyle(color: scheme.onSecondaryContainer),
            ),
            Align(
              alignment: Alignment.centerRight,
              child: TextButton(
                onPressed: () => _discard(context, ref, changes),
                child: const Text('Discard'),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _discard(
    BuildContext context,
    WidgetRef ref,
    String changes,
  ) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Discard unsynced changes?'),
        content: Text(
          '$changes will be deleted from this device and never reach the '
          'server.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Keep'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Discard'),
          ),
        ],
      ),
    );
    if (ok ?? false) await ref.read(outboxProvider).clearOthers();
  }
}
