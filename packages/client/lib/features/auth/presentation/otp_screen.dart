import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/auth/data/providers.dart';
import 'package:splitspends/features/sync/data/providers.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';
import 'package:splitspends/features/auth/presentation/auth_layout.dart';
import 'package:splitspends/features/auth/presentation/otp_input.dart';
import 'package:splitspends/features/invites/presentation/providers.dart';

/// Step 2: enter the emailed code; step 3 (new accounts): your name.
class OtpScreen extends ConsumerStatefulWidget {
  const OtpScreen({super.key, required this.email});

  final String email;

  @override
  ConsumerState<OtpScreen> createState() => _OtpScreenState();
}

class _OtpScreenState extends ConsumerState<OtpScreen> {
  static const _resendSeconds = 30;

  final _code = TextEditingController();
  final _name = TextEditingController();
  Timer? _timer;
  int _cooldown = _resendSeconds;
  bool _busy = false;
  bool _resending = false;
  String? _error;
  LoginResult? _pending;

  /// Set when the server says the account waits for approval (or was
  /// declined); the screen then explains instead of asking for a code.
  ApiError? _waiting;

  String get _server => ref.read(sessionProvider).value?.serverUrl ?? '';

  @override
  void initState() {
    super.initState();
    _startCooldown();
  }

  @override
  void dispose() {
    _timer?.cancel();
    _code.dispose();
    _name.dispose();
    super.dispose();
  }

  void _startCooldown() {
    _timer?.cancel();
    setState(() => _cooldown = _resendSeconds);
    _timer = Timer.periodic(const Duration(seconds: 1), (timer) {
      if (!mounted) return;
      setState(() => _cooldown--);
      if (_cooldown <= 0) timer.cancel();
    });
  }

  Future<void> _verify(String code) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final result = await ref
          .read(authRepositoryProvider)
          .verifyCode(_server, widget.email, code);
      final session = ref.read(sessionProvider).value;
      final localName = (session?.isLocal ?? false) ? session?.me?.name : null;
      var login = result;
      final needsName = login.isNew || login.me.name.trim().isEmpty;
      if (needsName && localName != null && localName.trim().isNotEmpty) {
        // New account from on-device mode: reuse the name given there.
        final me = await ref
            .read(authRepositoryProvider)
            .setName(_server, login.token, localName);
        login = LoginResult(token: login.token, me: me, isNew: login.isNew);
      } else if (needsName) {
        _name.text = login.me.name;
        setState(() => _pending = login);
        return;
      }
      await _continue(login);
    } catch (error) {
      if (!mounted) return;
      _code.clear();
      if (error is ApiError && (error.isPendingApproval || error.isDeclined)) {
        setState(() => _waiting = error);
        return;
      }
      setState(() => _error = describeError(error));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// Signs in, or first offers to upload on-device data.
  Future<void> _continue(LoginResult login) async {
    final local = ref.read(sessionProvider).value?.isLocal ?? false;
    if (local) {
      final summary = await ref.read(importServiceProvider).summary();
      if (!summary.isEmpty && mounted) {
        context.go(Routes.importData, extra: login);
        return;
      }
    }
    await ref
        .read(sessionProvider.notifier)
        .signIn(token: login.token, me: login.me);
  }

  /// Starts the cooldown before asking, and keeps it when the request
  /// fails: every request may send an email and counts towards the
  /// server's hourly limit, so a slow or failed send must not invite more.
  Future<void> _resend() async {
    _startCooldown();
    setState(() => _resending = true);
    try {
      await ref
          .read(authRepositoryProvider)
          .requestCode(
            _server,
            widget.email,
            inviteToken: ref.read(pendingInviteProvider)?.token,
          );
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(const SnackBar(content: Text('New code sent')));
      }
    } catch (error) {
      if (!mounted) return;
      setState(() => _error = describeError(error));
    } finally {
      if (mounted) setState(() => _resending = false);
    }
  }

  Future<void> _saveName() async {
    final pending = _pending!;
    final name = _name.text.trim();
    if (name.isEmpty) {
      setState(() => _error = 'Tell your friends who you are.');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final me = await ref
          .read(authRepositoryProvider)
          .setName(_server, pending.token, name);
      await _continue(
        LoginResult(token: pending.token, me: me, isNew: pending.isNew),
      );
    } catch (error) {
      if (!mounted) return;
      setState(() => _error = describeError(error));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final error = _error == null
        ? const SizedBox.shrink()
        : Padding(
            padding: const EdgeInsets.only(top: 16),
            child: Text(_error!, style: TextStyle(color: scheme.error)),
          );

    final waiting = _waiting;
    if (waiting != null) {
      return AuthLayout(
        key: const Key('approval-waiting'),
        title: waiting.isDeclined ? 'Request declined' : 'Waiting for approval',
        subtitle: waiting.message,
        children: [
          Icon(
            waiting.isDeclined
                ? Icons.block_rounded
                : Icons.hourglass_top_rounded,
            size: 56,
            color: Theme.of(context).colorScheme.primary,
          ),
          const SizedBox(height: 16),
          if (!waiting.isDeclined)
            Text(
              'You will get an email when your account is approved. '
              'Then sign in again with the same address.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyMedium,
            ),
          const SizedBox(height: 24),
          FilledButton.tonal(
            onPressed: () => context.go(Routes.login),
            child: const Text('Back to sign in'),
          ),
        ],
      );
    }
    if (_pending != null) {
      return AuthLayout(
        title: 'Welcome aboard',
        subtitle: 'What should your friends call you?',
        children: [
          TextField(
            key: const Key('name-field'),
            controller: _name,
            autofocus: true,
            textCapitalization: TextCapitalization.words,
            autofillHints: const [AutofillHints.name],
            textInputAction: TextInputAction.done,
            onSubmitted: (_) => _saveName(),
            decoration: const InputDecoration(
              labelText: 'Your name',
              prefixIcon: Icon(Icons.badge_outlined),
            ),
          ),
          error,
          const SizedBox(height: 24),
          FilledButton(
            onPressed: _busy ? null : _saveName,
            child: const Text('Get started'),
          ),
        ],
      );
    }

    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) context.go(Routes.login);
      },
      child: AuthLayout(
        showBack: true,
        title: 'Check your inbox',
        subtitle: 'Enter the 6-digit code we sent to ${widget.email}.',
        children: [
          OtpInput(controller: _code, enabled: !_busy, onCompleted: _verify),
          error,
          const SizedBox(height: 24),
          SizedBox(
            height: 4,
            child: _busy ? const LinearProgressIndicator() : null,
          ),
          const SizedBox(height: 12),
          Wrap(
            alignment: WrapAlignment.spaceBetween,
            children: [
              TextButton(
                onPressed: _busy ? null : () => context.go(Routes.login),
                child: const Text('Use another email'),
              ),
              TextButton(
                onPressed: _cooldown > 0 || _busy || _resending
                    ? null
                    : _resend,
                child: Text(
                  _cooldown > 0 ? 'Resend in ${_cooldown}s' : 'Resend code',
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
