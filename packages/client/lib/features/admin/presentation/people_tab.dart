import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/admin/domain/admin.dart';
import 'package:splitspends/features/admin/presentation/providers.dart';

const _roles = {
  'user': 'User',
  'manager': 'Manager (approves people)',
  'admin': 'Administrator (everything)',
};

/// What the account sheet asks for.
sealed class _Choice {
  const _Choice();
}

final class _Role extends _Choice {
  const _Role(this.role);

  final String role;
}

final class _Enable extends _Choice {
  const _Enable();
}

final class _Disable extends _Choice {
  const _Disable();
}

/// Every account on the server, with roles and disabling.
class PeopleTab extends ConsumerStatefulWidget {
  const PeopleTab({super.key});

  @override
  ConsumerState<PeopleTab> createState() => _PeopleTabState();
}

class _PeopleTabState extends ConsumerState<PeopleTab> {
  final _search = TextEditingController();
  String _status = '';

  /// A change is on its way; another would race it.
  bool _busy = false;

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  /// Runs [action] and returns its result, or null when it failed (the
  /// error is shown).
  Future<T?> _run<T>(Future<T> Function() action, String done) async {
    if (_busy) return null;
    // Switching tabs disposes this one while the change is on its way.
    final container = ProviderScope.containerOf(context, listen: false);
    setState(() => _busy = true);
    try {
      final result = await action();
      container.invalidate(accountsProvider);
      if (mounted) showMessage(context, done);
      return result;
    } catch (error) {
      if (mounted) showError(context, error);
      return null;
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _manage(Account a) async {
    final choice = await _choose(a);
    if (!mounted) return;
    final repo = ref.read(adminRepositoryProvider);
    final name = a.user.displayName;
    switch (choice) {
      case null:
        return;
      case _Role(:final role):
        if (role != a.role) await _setRole(a, role);
      case _Enable():
        await _run(() => repo.enable(a.user.id), '$name can sign in again');
      case _Disable():
        if (await _confirmDisable(a) && mounted) {
          await _run(() => repo.disable(a.user.id), '$name is disabled');
        }
    }
  }

  Future<void> _setRole(Account a, String role) async {
    final session = ref.read(sessionProvider.notifier);
    final me = ref.read(meProvider);
    final saved = await _run(
      () => ref.read(adminRepositoryProvider).setRole(a.user.id, role),
      '${a.user.displayName} is now '
      '${_roles[role]!.split(' ').first.toLowerCase()}',
    );
    // Stepping down takes effect here at once: the screen follows the new
    // role instead of offering actions the server now refuses.
    if (saved != null && me != null && me.id == a.user.id) {
      await session.updateMe(me.copyWith(role: saved.role));
    }
  }

  Future<_Choice?> _choose(Account a) {
    final meId = ref.read(meProvider)?.id;
    return showModalBottomSheet<_Choice>(
      context: context,
      useRootNavigator: true,
      showDragHandle: true,
      builder: (context) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ListTile(
              leading: UserAvatar(user: a.user),
              title: Text(a.user.displayName),
              subtitle: Text(a.user.email ?? 'Email removed (disabled)'),
            ),
            const Divider(),
            if (!a.disabled)
              for (final entry in _roles.entries)
                RadioListTile<String>(
                  value: entry.key,
                  // ignore: deprecated_member_use
                  groupValue: a.role,
                  title: Text(entry.value),
                  // ignore: deprecated_member_use
                  onChanged: (v) =>
                      Navigator.pop(context, v == null ? null : _Role(v)),
                ),
            if (a.user.id != meId)
              ListTile(
                leading: Icon(
                  a.disabled ? Icons.lock_open_rounded : Icons.block_rounded,
                  color: a.disabled
                      ? null
                      : Theme.of(context).colorScheme.error,
                ),
                title: Text(a.disabled ? 'Enable account' : 'Disable account'),
                subtitle: Text(
                  a.disabled
                      ? 'They can sign in again with their old email.'
                      : 'Signs them out everywhere and removes their email. '
                            'Their name stays on past expenses.',
                ),
                onTap: () => Navigator.pop(
                  context,
                  a.disabled ? const _Enable() : const _Disable(),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Future<bool> _confirmDisable(Account a) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Disable ${a.user.displayName}?'),
        content: const Text(
          'They are signed out on every device, their API tokens and '
          'push devices are removed, and their email address is deleted '
          '(only a hash is kept so the same address is recognised). '
          'Their name stays on shared expenses. You can enable them again.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            key: const Key('confirm-disable'),
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Disable'),
          ),
        ],
      ),
    );
    return ok ?? false;
  }

  @override
  Widget build(BuildContext context) {
    final filter = (status: _status, query: _search.text.trim());
    final accounts = ref.watch(accountsProvider(filter));
    final scheme = Theme.of(context).colorScheme;
    return ListView(
      padding: const EdgeInsets.only(bottom: 48),
      children: [
        MaxWidth(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
            child: TextField(
              controller: _search,
              textInputAction: TextInputAction.search,
              onSubmitted: (_) => setState(() {}),
              decoration: InputDecoration(
                hintText: 'Search names and emails',
                prefixIcon: const Icon(Icons.search_rounded),
                suffixIcon: IconButton(
                  tooltip: 'Search',
                  icon: const Icon(Icons.arrow_forward_rounded),
                  onPressed: () => setState(() {}),
                ),
              ),
            ),
          ),
        ),
        MaxWidth(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Wrap(
              spacing: 8,
              children: [
                for (final (value, label) in const [
                  ('', 'All'),
                  ('active', 'Active'),
                  ('invited', 'Not signed in yet'),
                  ('disabled', 'Disabled'),
                ])
                  ChoiceChip(
                    label: Text(label),
                    selected: _status == value,
                    onSelected: (_) => setState(() => _status = value),
                  ),
              ],
            ),
          ),
        ),
        ...accounts.when(
          loading: () => [const LinearProgressIndicator()],
          error: (error, _) => [
            ErrorView(
              error: error,
              onRetry: () => ref.invalidate(accountsProvider),
            ),
          ],
          data: (page) => [
            for (final a in page.items)
              MaxWidth(
                child: ListTile(
                  key: Key('account-${a.user.id}'),
                  leading: UserAvatar(user: a.user),
                  title: Text(a.user.displayName),
                  subtitle: Text(
                    a.disabled
                        ? 'Disabled'
                        : [
                            a.user.email ?? '',
                            if (a.user.status == 'invited') 'not signed in yet',
                          ].where((s) => s.isNotEmpty).join(' · '),
                  ),
                  trailing: a.role == 'user'
                      ? const Icon(Icons.chevron_right_rounded)
                      : Chip(
                          label: Text(a.role == 'admin' ? 'Admin' : 'Manager'),
                          backgroundColor: scheme.secondaryContainer,
                          visualDensity: VisualDensity.compact,
                        ),
                  onTap: _busy ? null : () => _manage(a),
                ),
              ),
            if (page.hasMore)
              MaxWidth(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Text(
                    'Showing ${page.items.length} of ${page.total}. '
                    'Search to narrow the list.',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ),
              ),
          ],
        ),
      ],
    );
  }
}
