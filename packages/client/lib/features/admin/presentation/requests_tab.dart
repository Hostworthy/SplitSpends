import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/admin/domain/admin.dart';
import 'package:splitspends/features/admin/presentation/providers.dart';

/// Sign-up requests by status, approved or declined here.
class RequestsTab extends ConsumerStatefulWidget {
  const RequestsTab({super.key});

  @override
  ConsumerState<RequestsTab> createState() => _RequestsTabState();
}

class _RequestsTabState extends ConsumerState<RequestsTab> {
  String _status = 'pending';

  /// A decision is on its way; a second tap would send it twice.
  bool _busy = false;

  void _refresh() => ref.invalidate(signupRequestsProvider);

  Future<void> _run(Future<void> Function() action, String done) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      await action();
      _refresh();
      if (mounted) showMessage(context, done);
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _approve(SignupRequest r) => _run(
    () => ref.read(adminRepositoryProvider).approve(r.id),
    '${r.label} can now sign in',
  );

  Future<void> _decline(SignupRequest r) => _run(
    () => ref.read(adminRepositoryProvider).decline(r.id),
    '${r.label} declined',
  );

  @override
  Widget build(BuildContext context) {
    final requests = ref.watch(signupRequestsProvider(_status));
    return ListView(
      padding: const EdgeInsets.only(bottom: 48),
      children: [
        MaxWidth(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: SegmentedButton<String>(
              segments: const [
                ButtonSegment(value: 'pending', label: Text('Waiting')),
                ButtonSegment(value: 'declined', label: Text('Declined')),
                ButtonSegment(value: 'approved', label: Text('Approved')),
              ],
              selected: {_status},
              onSelectionChanged: (v) => setState(() => _status = v.first),
            ),
          ),
        ),
        ...requests.when(
          loading: () => [const LinearProgressIndicator()],
          error: (error, _) => [ErrorView(error: error, onRetry: _refresh)],
          data: (list) => list.isEmpty
              ? [
                  EmptyState(
                    icon: Icons.how_to_reg_outlined,
                    title: _status == 'pending'
                        ? 'Nobody is waiting'
                        : 'Nothing here',
                    message: _status == 'pending'
                        ? 'In approval mode, people who sign up appear here.'
                        : 'Decided requests appear here.',
                  ),
                ]
              : [for (final r in list) _card(r)],
        ),
      ],
    );
  }

  Widget _card(SignupRequest r) => MaxWidth(
    child: Card(
      key: Key('request-${r.email}'),
      margin: const EdgeInsets.fromLTRB(16, 0, 16, 12),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(0, 4, 12, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ListTile(
              leading: const CircleAvatar(
                child: Icon(Icons.person_outline_rounded),
              ),
              title: Text(r.label),
              subtitle: Text(
                [
                  if (r.name.trim().isNotEmpty) r.email,
                  'Asked ${DateFormat.yMMMd().format(r.requestedAt.toLocal())}',
                ].join('\n'),
              ),
            ),
            Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                if (r.status != 'declined')
                  TextButton(
                    onPressed: _busy ? null : () => _decline(r),
                    child: const Text('Decline'),
                  ),
                const SizedBox(width: 8),
                if (r.status != 'approved')
                  FilledButton.tonal(
                    onPressed: _busy ? null : () => _approve(r),
                    child: const Text('Approve'),
                  ),
              ],
            ),
          ],
        ),
      ),
    ),
  );
}
