import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/features/push/presentation/providers.dart';

/// Signs out at the user's request. Changes that have not reached the
/// server are discarded, so it asks first when there are any. When the
/// server ends the session instead, push stops on this device through
/// pushSignOutProvider.
Future<void> signOut(BuildContext context, WidgetRef ref) async {
  final waiting = (await ref.read(outboxProvider).pending()).length;
  if (!context.mounted) return;
  if (waiting > 0 && !await _confirm(context, waiting)) return;
  // This device should stop getting the account's notifications; the
  // server is told while the session still works.
  await ref.read(pushActionsProvider).forget();
  await ref.read(sessionProvider.notifier).signOut();
}

Future<bool> _confirm(BuildContext context, int waiting) async {
  final ok = await showDialog<bool>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('Sign out anyway?'),
      content: Text(
        '$waiting change${waiting == 1 ? ' has' : 's have'} not reached '
        'the server yet. Signing out discards '
        '${waiting == 1 ? 'it' : 'them'}.',
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context, false),
          child: const Text('Stay signed in'),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context, true),
          child: const Text('Sign out'),
        ),
      ],
    ),
  );
  return ok ?? false;
}
