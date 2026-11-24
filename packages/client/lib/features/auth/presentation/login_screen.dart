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
    if (!_emailPattern.hasMatch(email)) {
      setState(() => _error = 'Enter a valid email address.');
      return;
    }
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
    } on ApiError catch (error) {
      setState(() => _error = describeError(error));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
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

/// True for `http://` addresses, which send sign-in codes unencrypted.
bool insecureServer(String url) =>
    url.trim().toLowerCase().startsWith('http://');
