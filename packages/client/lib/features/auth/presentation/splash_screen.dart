import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/shell.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/sync/presentation/save_backup.dart';

/// Shown while the session loads, and with a way out when it cannot.
class SplashScreen extends ConsumerWidget {
  const SplashScreen({super.key});

  void _retry(WidgetRef ref) {
    // A database that failed to open stays failed until it is reopened.
    if (ref.read(databaseProvider).hasError) ref.invalidate(databaseProvider);
    ref.invalidate(sessionProvider);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    if (!session.hasError || session.isLoading) {
      return const Scaffold(body: Center(child: AppLogo(size: 56)));
    }
    return Scaffold(
      body: SafeArea(
        child: EmptyState(
          icon: Icons.error_outline_rounded,
          title: 'Could not open your data',
          message:
              '${describeError(session.error!)} Try again, or save a backup '
              'of what is stored on this device.',
          action: Wrap(
            spacing: 8,
            runSpacing: 8,
            alignment: WrapAlignment.center,
            children: [
              FilledButton.tonalIcon(
                onPressed: () => _retry(ref),
                icon: const Icon(Icons.refresh_rounded),
                label: const Text('Try again'),
              ),
              OutlinedButton.icon(
                onPressed: () => saveBackup(context, ref),
                icon: const Icon(Icons.download_rounded),
                label: const Text('Save backup'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
