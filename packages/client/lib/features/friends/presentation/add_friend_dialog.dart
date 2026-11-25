import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/features/friends/data/providers.dart';
import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/widgets/person_dialog.dart';
import 'package:splitspends/core/widgets/states.dart';

/// Asks for a name and/or email and adds the friend.
Future<void> showAddFriendDialog(BuildContext context) async {
  final container = ProviderScope.containerOf(context);
  final entry = await showPersonDialog(context, title: 'Add a friend');
  if (entry == null || !context.mounted) return;
  try {
    final friend = await container
        .read(friendsRepositoryProvider)
        .add(email: entry.email, name: entry.name);
    refreshLedgerIn(container);
    if (!context.mounted) return;
    GoRouter.of(context).go(Routes.friend(friend.user.id));
    showMessage(
      context,
      friend.user.isInvited
          ? 'Invite sent to ${friend.user.email}'
          : '${friend.user.displayName} added',
    );
  } catch (error) {
    if (context.mounted) showError(context, error);
  }
}

/// Adds an email to someone added by name (invites them when connected).
Future<void> showInviteDialog(
  BuildContext context,
  String userId,
  String name,
) async {
  final container = ProviderScope.containerOf(context);
  final entry = await showPersonDialog(
    context,
    title: 'Add $name’s email',
    action: 'Save',
    emailOnly: true,
  );
  if (entry?.email == null || !context.mounted) return;
  try {
    final friend = await container
        .read(friendsRepositoryProvider)
        .invite(userId, entry!.email!);
    refreshLedgerIn(container);
    if (context.mounted) {
      showMessage(
        context,
        friend.user.isInvited ? 'Invite sent to ${entry.email}' : 'Email saved',
      );
    }
  } catch (error) {
    if (context.mounted) showError(context, error);
  }
}
