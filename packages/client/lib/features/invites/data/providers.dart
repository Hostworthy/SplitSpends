import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/invites/data/remote_invites_repository.dart';
import 'package:splitspends/features/invites/domain/invite.dart';

/// Invite links exist only on a server.
final invitesRepositoryProvider = Provider<InvitesRepository>(
  (ref) => byMode(ref, remote: RemoteInvitesRepository.new),
);

/// Link previews are public, so they work before signing in.
final invitePreviewsProvider = Provider<InvitePreviews>(
  (ref) => RemoteInvitePreviews(ref.read(httpClientProvider)),
);
