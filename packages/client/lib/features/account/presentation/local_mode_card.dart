import 'package:file_picker/file_picker.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/features/sync/data/providers.dart';
import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/sync/domain/import_payload.dart';
import 'package:splitspends/features/sync/presentation/save_backup.dart';

final _localSummaryProvider = FutureProvider.autoDispose<ImportSummary>(
  (ref) => ref.watch(importServiceProvider).summary(),
);

/// Account card in on-device mode: where data lives, backups, connecting.
class LocalModeCard extends ConsumerWidget {
  const LocalModeCard({super.key});

  Future<void> _restore(BuildContext context) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Restore a backup?'),
        content: const Text(
          'This replaces everything on this device with the backup file.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Choose file'),
          ),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    // The restore outlives this card when the person leaves meanwhile; the
    // session and lists must reload either way.
    final container = ProviderScope.containerOf(context, listen: false);
    try {
      final file = await FilePicker.pickFile(
        type: FileType.custom,
        allowedExtensions: const ['json'],
      );
      if (file == null) return;
      await container
          .read(backupServiceProvider)
          .restore(await file.xFile.readAsBytes());
      container.invalidate(sessionProvider);
      refreshLedgerIn(container);
      if (context.mounted) showMessage(context, 'Backup restored');
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final scheme = Theme.of(context).colorScheme;
    final text = Theme.of(context).textTheme;
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 20, 16, 0),
      color: scheme.secondaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.smartphone_rounded,
                  color: scheme.onSecondaryContainer,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    'Using Split Spends on this device',
                    style: text.titleMedium,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'Your data lives only in this browser or phone. Clearing the '
              'app’s or site’s data deletes it, so export a backup now and then.',
              style: text.bodyMedium,
            ),
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                OutlinedButton.icon(
                  onPressed: () => saveBackup(context, ref),
                  icon: const Icon(Icons.download_rounded),
                  label: const Text('Export backup'),
                ),
                OutlinedButton.icon(
                  onPressed: () => _restore(context),
                  icon: const Icon(Icons.restore_rounded),
                  label: const Text('Restore backup'),
                ),
              ],
            ),
            const Divider(height: 28),
            Text(
              'Connect to a server to sync between devices, share groups and '
              'get email updates. You can upload what you have here.',
              style: text.bodyMedium,
            ),
            const SizedBox(height: 12),
            FilledButton.tonalIcon(
              key: const Key('connect-from-local'),
              onPressed: () => context.push(Routes.login),
              icon: const Icon(Icons.cloud_sync_rounded),
              label: const Text('Connect to a server'),
            ),
          ],
        ),
      ),
    );
  }
}

/// Connected mode: the on-device copy kept after connecting, until removed.
class LocalCopyCard extends ConsumerWidget {
  const LocalCopyCard({super.key});

  Future<void> _remove(BuildContext context) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Remove the on-device copy?'),
        content: const Text(
          'This deletes the groups, expenses and people stored only on this '
          'device. Your account on the server is not affected.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Keep'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Remove'),
          ),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    final container = ProviderScope.containerOf(context, listen: false);
    try {
      await container.read(importServiceProvider).removeLocalCopy();
      container.invalidate(_localSummaryProvider);
      if (context.mounted) showMessage(context, 'On-device copy removed');
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final summary = ref.watch(_localSummaryProvider).value;
    if (summary == null || summary.isEmpty) return const SizedBox.shrink();
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 20, 16, 0),
      child: ListTile(
        leading: const Icon(Icons.phonelink_lock_rounded),
        title: const Text('On-device copy'),
        subtitle: Text(
          '${summary.groups} groups, ${summary.expenses} expenses and '
          '${summary.people} people from before you connected.',
        ),
        trailing: TextButton(
          onPressed: () => _remove(context),
          child: const Text('Remove'),
        ),
      ),
    );
  }
}
