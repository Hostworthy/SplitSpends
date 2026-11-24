import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';
import 'package:splitspends/features/auth/presentation/auth_layout.dart';
import 'package:splitspends/features/sync/domain/import_payload.dart';

final _summaryProvider = FutureProvider.autoDispose<ImportSummary>(
  (ref) => ref.watch(importServiceProvider).summary(),
);

/// After signing in from on-device mode: upload local data, or start fresh.
/// Either way the local copy stays on this device.
class ImportScreen extends ConsumerStatefulWidget {
  const ImportScreen({super.key, required this.login});

  final LoginResult login;

  @override
  ConsumerState<ImportScreen> createState() => _ImportScreenState();
}

class _ImportScreenState extends ConsumerState<ImportScreen> {
  bool _busy = false;
  String? _error;

  Future<void> _finish() => ref
      .read(sessionProvider.notifier)
      .signIn(token: widget.login.token, me: widget.login.me);

  Future<void> _upload() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final server = ref.read(sessionProvider).value?.serverUrl ?? '';
      await ref
          .read(importServiceProvider)
          .upload(serverUrl: server, token: widget.login.token);
      await _finish();
    } on ApiError catch (error) {
      setState(
        () => _error = error.field == null
            ? describeError(error)
            : '${describeError(error)} (${error.field})',
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final summary = ref.watch(_summaryProvider).value;
    final scheme = Theme.of(context).colorScheme;
    String count(int n, String one, String many) => '$n ${n == 1 ? one : many}';
    final what = summary == null
        ? 'your on-device data'
        : '${count(summary.groups, 'group', 'groups')}, '
              '${count(summary.expenses, 'expense', 'expenses')} and '
              '${count(summary.people, 'person', 'people')}';
    return AuthLayout(
      title: 'Bring your data along?',
      subtitle:
          'Upload $what from this device to your account? People you added '
          'with an email are matched to their accounts; the rest are added '
          'by name. Nothing is emailed during the upload.',
      children: [
        if (_error != null) ...[
          Text(_error!, style: TextStyle(color: scheme.error)),
          const SizedBox(height: 16),
        ],
        FilledButton.icon(
          key: const Key('import-upload'),
          onPressed: _busy ? null : _upload,
          icon: _busy
              ? const SizedBox.square(
                  dimension: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.cloud_upload_rounded),
          label: const Text('Upload'),
        ),
        const SizedBox(height: 12),
        OutlinedButton(
          key: const Key('import-skip'),
          onPressed: _busy ? null : _finish,
          child: const Text('Start fresh (keep local copy)'),
        ),
        const SizedBox(height: 16),
        Text(
          'Either way, the on-device copy stays here until you remove it '
          'in Account.',
          style: Theme.of(context).textTheme.bodySmall,
        ),
      ],
    );
  }
}
