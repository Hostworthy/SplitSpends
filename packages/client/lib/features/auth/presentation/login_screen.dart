import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/auth/presentation/auth_layout.dart';
import 'package:splitspends/features/auth/presentation/waiting_changes.dart';
import 'package:splitspends/features/invites/presentation/providers.dart';

final _emailPattern = RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$');

/// Step 1: choose the server (mobile) and enter an email address.
class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _email = TextEditingController();
  final _server = TextEditingController();
  bool _editServer = false;
  bool _busy = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    final session = ref.read(sessionProvider).value;
    _server.text = session?.serverUrl ?? defaultServerUrl();
    _editServer = _server.text.isEmpty;
  }

  @override
  void dispose() {
    _email.dispose();
    _server.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final email = _email.text.trim().toLowerCase();
    final server = normalizeServerUrl(_server.text);
    if (server.isEmpty) {
      setState(() => _error = 'Enter your Split Spends server address.');
      return;
    }
    if (!_validServer(server)) {
      setState(() => _error = 'Enter a valid server address.');
      return;
    }
    if (!_emailPattern.hasMatch(email)) {
      setState(() => _error = 'Enter a valid email address.');
      return;
    }
    if (publicPlainServer(server) && !await _confirmPlain()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final session = ref.read(sessionProvider.notifier);
      if (ref.read(sessionProvider).value?.serverUrl != server) {
        await session.setServerUrl(server);
      }
      await ref
          .read(authRepositoryProvider)
          .requestCode(
            server,
            email,
            inviteToken: ref.read(pendingInviteProvider)?.token,
          );
      if (!mounted) return;
      context.go('${Routes.verify}?email=${Uri.encodeQueryComponent(email)}');
    } catch (error) {
      if (!mounted) return;
      setState(() => _error = describeError(error));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// Asks before sending a code and a session over plain HTTP outside the
  /// local network.
  Future<bool> _confirmPlain() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('This server is not encrypted'),
        content: const Text(
          'Your sign-in code and session would travel unencrypted, so '
          'anyone on the network between you and the server could read '
          'them and use your account. Use https:// outside your home '
          'network.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Continue anyway'),
          ),
        ],
      ),
    );
    return ok ?? false;
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final invited = ref.watch(pendingInviteProvider) != null;
    return AuthLayout(
      showBack: true,
      title: 'Connect to a server',
      subtitle: invited
          ? 'Sign in with your email to join the group. We will email you a '
                '6-digit code.'
          : 'Sign in with your email to sync and share. We will email you a 6-digit code.',
      children: [
        const WaitingChanges(),
        TextField(
          key: const Key('login-email'),
          controller: _email,
          autofocus: true,
          enabled: !_busy,
          keyboardType: TextInputType.emailAddress,
          autofillHints: const [AutofillHints.email],
          textInputAction: TextInputAction.go,
          onSubmitted: (_) => _submit(),
          decoration: const InputDecoration(
            labelText: 'Email',
            prefixIcon: Icon(Icons.alternate_email_rounded),
          ),
        ),
        const SizedBox(height: 12),
        AnimatedSize(
          duration: const Duration(milliseconds: 200),
          child: _editServer
              ? TextField(
                  key: const Key('login-server'),
                  controller: _server,
                  enabled: !_busy,
                  keyboardType: TextInputType.url,
                  textInputAction: TextInputAction.go,
                  onSubmitted: (_) => _submit(),
                  onChanged: (_) => setState(() {}),
                  decoration: InputDecoration(
                    labelText: 'Server',
                    hintText: 'https://splitspends.example.com',
                    prefixIcon: const Icon(Icons.dns_outlined),
                    helperText: insecureServer(_server.text)
                        ? 'Not encrypted. Fine on your home network; use '
                              'https:// anywhere else.'
                        : null,
                    helperMaxLines: 2,
                  ),
                )
              : Align(
                  alignment: Alignment.centerLeft,
                  child: TextButton.icon(
                    onPressed: () => setState(() => _editServer = true),
                    icon: const Icon(Icons.dns_outlined, size: 18),
                    label: Text(
                      kIsWeb && _server.text == defaultServerUrl()
                          ? 'This server · change'
                          : '${Uri.tryParse(_server.text)?.host ?? _server.text} · change',
                    ),
                  ),
                ),
        ),
        if (_error != null) ...[
          const SizedBox(height: 12),
          Text(_error!, style: TextStyle(color: scheme.error)),
        ],
        const SizedBox(height: 24),
        FilledButton(
          key: const Key('login-continue'),
          onPressed: _busy ? null : _submit,
          child: _busy
              ? const SizedBox.square(
                  dimension: 22,
                  child: CircularProgressIndicator(strokeWidth: 2.5),
                )
              : const Text('Continue'),
        ),
      ],
    );
  }
}

/// True for an http or https address with a host; Uri.tryParse rejects
/// addresses such as an invalid port.
bool _validServer(String url) {
  final uri = Uri.tryParse(url);
  return uri != null &&
      (uri.isScheme('http') || uri.isScheme('https')) &&
      uri.host.isNotEmpty;
}

/// True for `http://` addresses, which send sign-in codes unencrypted.
bool insecureServer(String url) =>
    url.trim().toLowerCase().startsWith('http://');

/// True for an `http://` address outside the local network, where anyone
/// on the way could read the sign-in code and session token. The app
/// checks this itself: the phones' platform network rules do not govern
/// the HTTP client it uses.
bool publicPlainServer(String url) {
  if (!insecureServer(url)) return false;
  final host = Uri.tryParse(url.trim())?.host.toLowerCase() ?? '';
  return host.isNotEmpty && !_localHost(host);
}

/// Names that never resolve on the public internet: one label (resolved by
/// the local network), localhost, mDNS (.local) and the names reserved for
/// home and private networks and for testing.
const _localSuffixes = [
  '.localhost',
  '.local',
  '.home.arpa',
  '.internal',
  '.lan',
  '.test',
];

bool _localHost(String host) {
  if (host == 'localhost' || _localSuffixes.any(host.endsWith)) return true;
  final v4 = _ipv4(host);
  if (v4 != null) {
    final [a, b, _, _] = v4;
    return a == 127 || // loopback
        a == 10 ||
        (a == 172 && b >= 16 && b <= 31) ||
        (a == 192 && b == 168) ||
        (a == 169 && b == 254); // link-local
  }
  if (host.contains(':')) {
    try {
      final v6 = Uri.parseIPv6Address(host);
      final loopback = v6.take(15).every((b) => b == 0) && v6[15] == 1;
      return loopback ||
          (v6[0] & 0xfe) == 0xfc || // unique local, fc00::/7
          (v6[0] == 0xfe && (v6[1] & 0xc0) == 0x80); // link-local, fe80::/10
    } on FormatException {
      return false;
    }
  }
  return !host.contains('.');
}

/// The four parts of a dotted IPv4 address, or null.
List<int>? _ipv4(String host) {
  final parts = host.split('.');
  if (parts.length != 4) return null;
  final bytes = [for (final p in parts) int.tryParse(p)];
  if (bytes.any((b) => b == null || b < 0 || b > 255)) return null;
  return bytes.cast<int>();
}
