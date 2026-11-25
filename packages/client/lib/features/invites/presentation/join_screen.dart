import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/auth/presentation/auth_layout.dart';
import 'package:splitspends/features/auth/presentation/welcome_screen.dart';
import 'package:splitspends/features/invites/domain/invite.dart';
import 'package:splitspends/features/invites/presentation/providers.dart';

/// Opens an invite link (`/join/<token>`): shows the group, then joins it,
/// sending people without a session through sign-in first.
class JoinScreen extends ConsumerStatefulWidget {
  const JoinScreen({super.key, required this.token, this.server});

  final String token;

  /// The link's `?server=`, as given: pasted links for another server than
  /// the app's. Installed apps check it and ask before switching; the web
  /// build ignores it, since its own server serves the page.
  final String? server;

  @override
  ConsumerState<JoinScreen> createState() => _JoinScreenState();
}

/// What the person can do with a link, given their session.
enum JoinState { join, otherServer, onDevice, signIn }

/// Decides [JoinState] for a link to [server].
JoinState joinState(Session session, String server) {
  if (session.isConnected) {
    return normalizeServerUrl(session.serverUrl) == normalizeServerUrl(server)
        ? JoinState.join
        : JoinState.otherServer;
  }
  return session.isLocal ? JoinState.onDevice : JoinState.signIn;
}

class _JoinScreenState extends ConsumerState<JoinScreen> {
  bool _busy = false;
  bool _autoJoined = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    // A different link opened directly replaces an older pending one.
    final pending = ref.read(pendingInviteProvider);
    if (pending != null && pending.token != widget.token) {
      WidgetsBinding.instance.addPostFrameCallback(
        (_) => ref.read(pendingInviteProvider.notifier).set(null),
      );
    }
  }

  String _serverFor(Session session) =>
      ref.read(homeServerProvider) ??
      (widget.server == null ? null : JoinLink.origin(widget.server!)) ??
      session.serverUrl;

  /// True when the link names a server the app refuses to talk to.
  bool get _unsafe =>
      ref.read(homeServerProvider) == null &&
      widget.server != null &&
      JoinLink.origin(widget.server!) == null;

  Future<void> _join() async {
    // Joining completes even when the person leaves this screen meanwhile;
    // the pending invite must clear and the lists refresh either way.
    final container = ProviderScope.containerOf(context, listen: false);
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final group = await container
          .read(invitesRepositoryProvider)
          .accept(widget.token);
      container.read(pendingInviteProvider.notifier).set(null);
      refreshLedgerIn(container);
      if (mounted) context.go(Routes.group(group.id));
    } catch (error) {
      if (mounted) setState(() => _error = describeError(error));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// Sign in (or connect from on-device mode), then come back here. A link
  /// never moves the app to another server without the person's say-so.
  Future<void> _signIn(String server) async {
    final current = ref.read(sessionProvider).value?.serverUrl ?? '';
    final switching = normalizeServerUrl(current) != normalizeServerUrl(server);
    if (switching && !await _confirmServer(server, current)) return;
    if (!mounted) return;
    ref
        .read(pendingInviteProvider.notifier)
        .set(JoinLink(token: widget.token, server: server));
    if (switching) {
      await ref.read(sessionProvider.notifier).setServerUrl(server);
    }
    if (mounted) context.go(Routes.login);
  }

  Future<bool> _confirmServer(String server, String current) async {
    final host = Uri.tryParse(server)?.host ?? server;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(current.isEmpty ? 'Use this server?' : 'Switch servers?'),
        content: Text(
          'This invite is for $server. Split Spends will use that server '
          '${current.isEmpty ? 'from now on' : 'instead of $current'} and '
          'send your sign-in code to it. Continue only if you trust whoever '
          'sent you the link.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            key: const Key('join-confirm-server'),
            onPressed: () => Navigator.pop(context, true),
            child: Text('Use $host'),
          ),
        ],
      ),
    );
    return confirmed ?? false;
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(sessionProvider).value ?? const Session();
    if (_unsafe) {
      return AuthLayout(
        showBack: true,
        title: 'This link is not safe to open',
        subtitle:
            'It points to a server Split Spends will not use. Invite links '
            'start with https:// (plain http:// only works on your local '
            'network). Ask for a new link.',
        children: const [],
      );
    }
    final server = _serverFor(session);
    if (server.isEmpty) {
      return AuthLayout(
        showBack: true,
        title: 'Open the full link',
        subtitle:
            'This invite does not say which server it belongs to. Paste the '
            'whole link, starting with https://.',
        children: const [],
      );
    }
    final preview = ref.watch(
      invitePreviewProvider((server: server, token: widget.token)),
    );
    final state = joinState(session, server);
    final signedIn = Uri.tryParse(session.serverUrl)?.host ?? session.serverUrl;
    final pending = ref.watch(pendingInviteProvider);
    if (state == JoinState.join &&
        pending?.token == widget.token &&
        preview.hasValue &&
        !_autoJoined) {
      // Back from sign-in: finish what the person started.
      _autoJoined = true;
      WidgetsBinding.instance.addPostFrameCallback((_) => _join());
    }
    return switch (preview) {
      AsyncValue(:final value?) => _preview(
        context,
        value,
        state,
        server,
        signedIn,
      ),
      AsyncValue(:final error?) => _invalid(context, error),
      _ => const AuthLayout(
        title: 'Opening invite',
        subtitle: 'One moment…',
        children: [LinearProgressIndicator()],
      ),
    };
  }

  Widget _invalid(BuildContext context, Object error) {
    final network = error is ApiError && error.isNetwork;
    return AuthLayout(
      showBack: true,
      title: network ? 'Cannot reach the server' : 'This link does not work',
      subtitle: network
          ? 'Check your connection and try again.'
          : 'It may have expired or been revoked. Ask for a new invite link.',
      children: [
        if (network)
          FilledButton(
            onPressed: () => ref.invalidate(invitePreviewProvider),
            child: const Text('Try again'),
          ),
        TextButton(
          onPressed: () => context.go(Routes.home),
          child: const Text('Open Split Spends'),
        ),
      ],
    );
  }

  Widget _preview(
    BuildContext context,
    InvitePreview preview,
    JoinState state,
    String server,
    String signedIn,
  ) {
    final scheme = Theme.of(context).colorScheme;
    final text = Theme.of(context).textTheme;
    final color = parseHexColor(preview.color) ?? scheme.primary;
    final host = Uri.tryParse(server)?.host ?? server;
    final card = Card(
      key: const Key('join-preview'),
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        leading: CircleAvatar(
          radius: 26,
          backgroundColor: color.withValues(alpha: 0.18),
          child: Text(
            preview.emoji.isEmpty ? '👥' : preview.emoji,
            style: const TextStyle(fontSize: 24),
          ),
        ),
        title: Text(preview.groupName, style: text.titleMedium),
        subtitle: Text(
          '${preview.memberCount} '
          '${preview.memberCount == 1 ? 'member' : 'members'} · $host',
        ),
      ),
    );
    final error = _error == null
        ? const SizedBox.shrink()
        : Padding(
            padding: const EdgeInsets.only(top: 16),
            child: Text(_error!, style: TextStyle(color: scheme.error)),
          );
    final note = switch (state) {
      JoinState.join => 'You will see its expenses and balances right away.',
      JoinState.otherServer =>
        'This invite is for $host, but you are signed in to $signedIn. '
            'Sign out first to join with an account on $host.',
      JoinState.onDevice =>
        'Invite links need a Split Spends server. You are using Split Spends '
            'on this device only. Connect to the server to join, or set up '
            'your own.',
      JoinState.signIn =>
        'Sign in with your email to join. New to this server? The invite '
            'lets you create an account.',
    };
    return AuthLayout(
      showBack: true,
      title: 'Join ${preview.groupName}',
      subtitle:
          '${preview.invitedBy} invited you to share expenses in this '
          'group.',
      children: [
        card,
        const SizedBox(height: 16),
        Text(note, style: text.bodyMedium),
        error,
        const SizedBox(height: 24),
        ...switch (state) {
          JoinState.join => [
            FilledButton(
              key: const Key('join-accept'),
              onPressed: _busy ? null : _join,
              child: _busy
                  ? const SizedBox.square(
                      dimension: 22,
                      child: CircularProgressIndicator(strokeWidth: 2.5),
                    )
                  : const Text('Join group'),
            ),
          ],
          JoinState.otherServer => [
            OutlinedButton(
              onPressed: () => context.go(Routes.home),
              child: const Text('Open Split Spends'),
            ),
          ],
          JoinState.onDevice => [
            FilledButton(
              key: const Key('join-connect'),
              onPressed: () => _signIn(server),
              child: const Text('Connect to this server'),
            ),
            const SizedBox(height: 8),
            TextButton.icon(
              onPressed: () => launchUrl(hostingGuide),
              icon: const Icon(Icons.open_in_new_rounded, size: 18),
              label: const Text('How to host a server'),
            ),
          ],
          JoinState.signIn => [
            FilledButton(
              key: const Key('join-sign-in'),
              onPressed: () => _signIn(server),
              child: const Text('Sign in to join'),
            ),
          ],
        },
      ],
    );
  }
}
