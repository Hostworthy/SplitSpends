import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/admin/domain/admin.dart';
import 'package:splitspends/features/admin/presentation/providers.dart';

/// Administration for administrators (requests, people, sign-up) and
/// approvals only for managers. The server checks every action again.
class AdminScreen extends ConsumerWidget {
  const AdminScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final me = ref.watch(meProvider);
    if (me == null || !me.canApprove) {
      return Scaffold(
        appBar: AppBar(title: const Text('Administration')),
        body: const EmptyState(
          icon: Icons.lock_outline_rounded,
          title: 'Not available',
          message: 'Only administrators and managers can open this page.',
        ),
      );
    }
    if (!me.isAdmin) {
      return Scaffold(
        appBar: AppBar(title: const Text('Approvals')),
        body: const _Requests(),
      );
    }
    return DefaultTabController(
      length: 3,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Administration'),
          bottom: const TabBar(
            tabs: [
              Tab(text: 'Requests'),
              Tab(text: 'People'),
              Tab(text: 'Sign-up'),
            ],
          ),
        ),
        body: const TabBarView(children: [_Requests(), _People(), _Signup()]),
      ),
    );
  }
}

Future<void> _act(
  BuildContext context,
  Future<void> Function() action,
  String done,
) async {
  try {
    await action();
    if (context.mounted) showMessage(context, done);
  } catch (error) {
    if (context.mounted) showError(context, error);
  }
}

// ------------------------------------------------------------------ requests

class _Requests extends ConsumerStatefulWidget {
  const _Requests();

  @override
  ConsumerState<_Requests> createState() => _RequestsState();
}

class _RequestsState extends ConsumerState<_Requests> {
  String _status = 'pending';

  @override
  Widget build(BuildContext context) {
    final requests = ref.watch(signupRequestsProvider(_status));
    final repo = ref.read(adminRepositoryProvider);
    void refresh() => ref.invalidate(signupRequestsProvider);
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
          error: (error, _) => [ErrorView(error: error, onRetry: refresh)],
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
              : [
                  for (final r in list)
                    MaxWidth(
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
                                      onPressed: () => _act(context, () async {
                                        await repo.decline(r.id);
                                        refresh();
                                      }, '${r.label} declined'),
                                      child: const Text('Decline'),
                                    ),
                                  const SizedBox(width: 8),
                                  if (r.status != 'approved')
                                    FilledButton.tonal(
                                      onPressed: () => _act(context, () async {
                                        await repo.approve(r.id);
                                        refresh();
                                      }, '${r.label} can now sign in'),
                                      child: const Text('Approve'),
                                    ),
                                ],
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                ],
        ),
      ],
    );
  }
}

// -------------------------------------------------------------------- people

const _roles = {
  'user': 'User',
  'manager': 'Manager (approves people)',
  'admin': 'Administrator (everything)',
};

class _People extends ConsumerStatefulWidget {
  const _People();

  @override
  ConsumerState<_People> createState() => _PeopleState();
}

class _PeopleState extends ConsumerState<_People> {
  final _search = TextEditingController();
  String _status = '';

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  Future<void> _manage(Account a) async {
    final repo = ref.read(adminRepositoryProvider);
    final meId = ref.read(meProvider)?.id;
    void refresh() => ref.invalidate(accountsProvider);
    final choice = await showModalBottomSheet<String>(
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
                  onChanged: (v) => Navigator.pop(context, 'role:$v'),
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
                onTap: () =>
                    Navigator.pop(context, a.disabled ? 'enable' : 'disable'),
              ),
          ],
        ),
      ),
    );
    if (choice == null || !mounted) return;
    if (choice.startsWith('role:')) {
      final role = choice.substring(5);
      if (role == a.role) return;
      await _act(
        context,
        () async {
          await repo.setRole(a.user.id, role);
          refresh();
        },
        '${a.user.displayName} is now ${_roles[role]!.split(' ').first.toLowerCase()}',
      );
    } else if (choice == 'enable') {
      await _act(context, () async {
        await repo.enable(a.user.id);
        refresh();
      }, '${a.user.displayName} can sign in again');
    } else if (choice == 'disable') {
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
      if (ok != true || !mounted) return;
      await _act(context, () async {
        await repo.disable(a.user.id);
        refresh();
      }, '${a.user.displayName} is disabled');
    }
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
          data: (list) => [
            for (final a in list)
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
                  onTap: () => _manage(a),
                ),
              ),
          ],
        ),
      ],
    );
  }
}

// ------------------------------------------------------------------- sign-up

class _Signup extends ConsumerStatefulWidget {
  const _Signup();

  @override
  ConsumerState<_Signup> createState() => _SignupState();
}

class _SignupState extends ConsumerState<_Signup> {
  final _entry = TextEditingController();
  SignupMode? _mode;
  List<String>? _allowed;
  bool _saving = false;

  @override
  void dispose() {
    _entry.dispose();
    super.dispose();
  }

  void _add() {
    final value = _entry.text.trim().toLowerCase();
    if (value.isEmpty) return;
    setState(() {
      _allowed = {...?_allowed, value}.toList();
      _entry.clear();
    });
  }

  Future<void> _save(SignupSettings current) async {
    setState(() => _saving = true);
    try {
      final saved = await ref
          .read(adminRepositoryProvider)
          .saveSignup(
            SignupSettings(
              mode: _mode ?? current.mode,
              allowed: _allowed ?? current.allowed,
              unknownMode: _mode == null ? current.unknownMode : null,
            ),
          );
      ref.invalidate(signupSettingsProvider);
      setState(() {
        _mode = saved.mode;
        _allowed = saved.allowed;
      });
      if (mounted) showMessage(context, 'Sign-up settings saved');
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final settings = ref.watch(signupSettingsProvider);
    final text = Theme.of(context).textTheme;
    return settings.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (error, _) => ErrorView(
        error: error,
        onRetry: () => ref.invalidate(signupSettingsProvider),
      ),
      data: (current) {
        final mode = _mode ?? current.mode;
        final allowed = _allowed ?? current.allowed;
        final changed =
            mode != current.mode ||
            allowed.join(',') != current.allowed.join(',');
        return ListView(
          padding: const EdgeInsets.only(bottom: 48),
          children: [
            MaxWidth(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const SectionHeader('Who can sign up'),
                  if (mode == null)
                    Padding(
                      padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                      child: Text(
                        'This server uses a sign-up mode this app does not '
                        'know (${current.unknownMode ?? 'none'}). Saving the '
                        'allowed list keeps it; choose a mode below to '
                        'replace it, or update the app.',
                        style: text.bodyMedium,
                      ),
                    ),
                  for (final m in SignupMode.values)
                    RadioListTile<SignupMode>(
                      key: Key('mode-${m.name}'),
                      value: m,
                      // ignore: deprecated_member_use
                      groupValue: mode,
                      // ignore: deprecated_member_use
                      onChanged: (v) => setState(() => _mode = v),
                      title: Text(m.label),
                      subtitle: Text(m.description),
                    ),
                  const SectionHeader('Allowed list'),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    child: Text(
                      'Email addresses, or whole domains written like '
                      '@family.com. In invite and approval modes they sign '
                      'up without waiting. Closed mode ignores the list.',
                      style: text.bodySmall,
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
                    child: TextField(
                      key: const Key('allowed-entry'),
                      controller: _entry,
                      keyboardType: TextInputType.emailAddress,
                      onSubmitted: (_) => _add(),
                      decoration: InputDecoration(
                        hintText: 'name@example.com or @example.com',
                        suffixIcon: IconButton(
                          tooltip: 'Add to the list',
                          icon: const Icon(Icons.add_rounded),
                          onPressed: _add,
                        ),
                      ),
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    child: Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        if (allowed.isEmpty)
                          Text('The list is empty.', style: text.bodyMedium),
                        for (final entry in allowed)
                          InputChip(
                            label: Text(entry),
                            onDeleted: () => setState(
                              () => _allowed = [...allowed]..remove(entry),
                            ),
                          ),
                      ],
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.all(16),
                    child: FilledButton(
                      key: const Key('save-signup'),
                      onPressed: changed && !_saving
                          ? () => _save(current)
                          : null,
                      child: const Text('Save'),
                    ),
                  ),
                ],
              ),
            ),
          ],
        );
      },
    );
  }
}
