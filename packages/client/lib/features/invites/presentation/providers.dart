import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/invites/domain/invite.dart';

/// Active invite links of a group.
final groupInvitesProvider = FutureProvider.autoDispose
    .family<List<Invite>, String>(
      (ref, groupId) => ref.watch(invitesRepositoryProvider).list(groupId),
    );

/// The public preview of a link on a given server.
final invitePreviewProvider = FutureProvider.autoDispose
    .family<InvitePreview, ({String server, String token})>(
      (ref, key) =>
          ref.watch(invitePreviewsProvider).preview(key.server, key.token),
    );

/// An invite the user opened before signing in. Login passes its token to
/// the code request, and after signing in the app returns to the link.
final pendingInviteProvider = NotifierProvider<PendingInvite, JoinLink?>(
  PendingInvite.new,
);

class PendingInvite extends Notifier<JoinLink?> {
  @override
  JoinLink? build() => null;

  void set(JoinLink? link) => state = link;
}
