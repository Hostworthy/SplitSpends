import 'dart:convert';

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/download.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/person_dialog.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';
import 'package:splitspends/features/invites/presentation/invite_section.dart';

const _emojis = [
  '🏠',
  '✈️',
  '💞',
  '👥',
  '🏖️',
  '🎉',
  '🍕',
  '🚗',
  '🏕️',
  '⚽',
  '🎓',
  '💼',
];

class GroupSettingsScreen extends ConsumerWidget {
  const GroupSettingsScreen({super.key, required this.groupId});

  final String groupId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final group = ref.watch(groupProvider(groupId));
    return Scaffold(
      appBar: AppBar(title: const Text('Group settings')),
      body: AsyncView(
        value: group,
        onRetry: () => ref.invalidate(groupProvider(groupId)),
        data: (g) => MaxWidth(child: _Settings(group: g)),
      ),
    );
  }
}

class _Settings extends ConsumerStatefulWidget {
  const _Settings({required this.group});

  final Group group;

  @override
  ConsumerState<_Settings> createState() => _SettingsState();
}

class _SettingsState extends ConsumerState<_Settings> {
  late final _name = TextEditingController(text: widget.group.name);

  Group get group => widget.group;
  GroupsRepository get _repo => ref.read(groupsRepositoryProvider);

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _patch(GroupInput input, {String? done}) async {
    try {
      await _repo.update(group.id, input);
      ref.refreshLedger();
      if (mounted && done != null) showMessage(context, done);
    } catch (error) {
      if (mounted) showError(context, error);
    }
  }

  Future<void> _addMember() async {
    final entry = await showPersonDialog(context, title: 'Add member');
    if (entry == null) return;
    try {
      await _repo.addMember(group.id, email: entry.email, name: entry.name);
      ref.refreshLedger();
    } catch (error) {
      if (mounted) showError(context, error);
    }
  }

  Future<bool> _confirm(String title, String message, String action) async =>
      await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: Text(title),
          content: Text(message),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: Text(action),
            ),
          ],
        ),
      ) ??
      false;

  Future<void> _removeMember(String userId, {required bool self}) async {
    final ok = await _confirm(
      self ? 'Leave group?' : 'Remove member?',
      'Only possible when their balance in this group is settled.',
      self ? 'Leave' : 'Remove',
    );
    if (!ok) return;
    try {
      await _repo.removeMember(group.id, userId);
      ref.refreshLedger();
      if (self && mounted) context.go(Routes.groups);
    } catch (error) {
      if (mounted) showError(context, error);
    }
  }

  Future<void> _delete() async {
    final ok = await _confirm(
      'Delete group?',
      'This removes ${group.name} and its expenses for everyone. Only possible when everyone is settled up.',
      'Delete',
    );
    if (!ok) return;
    try {
      await _repo.delete(group.id);
      ref.refreshLedger();
      if (mounted) context.go(Routes.groups);
    } catch (error) {
      if (mounted) showError(context, error);
    }
  }

  Future<void> _export() async {
    try {
      final bytes = await _repo.export(group.id);
      final name = '${group.name.replaceAll(RegExp(r'[^\w\- ]'), '')}.csv';
      if (!saveDownload(name, bytes, 'text/csv')) {
        await Clipboard.setData(ClipboardData(text: utf8.decode(bytes)));
        if (mounted) showMessage(context, 'CSV copied to the clipboard');
      }
    } catch (error) {
      if (mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context) {
    final meId = ref.watch(meProvider)?.id;
    final scheme = Theme.of(context).colorScheme;
    return ListView(
      padding: const EdgeInsets.only(bottom: 96),
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
          child: TextField(
            controller: _name,
            textCapitalization: TextCapitalization.sentences,
            decoration: InputDecoration(
              labelText: 'Name',
              suffixIcon: IconButton(
                icon: const Icon(Icons.check_rounded),
                onPressed: () => _patch(
                  GroupInput(name: _name.text.trim()),
                  done: 'Renamed',
                ),
              ),
            ),
            onSubmitted: (v) =>
                _patch(GroupInput(name: v.trim()), done: 'Renamed'),
          ),
        ),
        const SectionHeader('Type'),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Wrap(
            spacing: 8,
            children: [
              for (final kind in GroupKind.values)
                ChoiceChip(
                  label: Text('${kind.emoji}  ${kind.label}'),
                  selected: group.kind == kind,
                  onSelected: (_) => _patch(GroupInput(kind: kind)),
                ),
            ],
          ),
        ),
        const SectionHeader('Look'),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              for (final e in _emojis)
                ChoiceChip(
                  label: Text(e, style: const TextStyle(fontSize: 18)),
                  selected: group.displayEmoji == e,
                  onSelected: (_) => _patch(GroupInput(emoji: e)),
                ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
          child: Wrap(
            spacing: 10,
            runSpacing: 10,
            children: [
              for (final c in palette)
                InkWell(
                  customBorder: const CircleBorder(),
                  onTap: () => _patch(GroupInput(color: toHexColor(c))),
                  child: CircleAvatar(
                    radius: 16,
                    backgroundColor: c,
                    child: group.color?.toUpperCase() == toHexColor(c)
                        ? const Icon(Icons.check, color: Colors.white, size: 18)
                        : null,
                  ),
                ),
            ],
          ),
        ),
        const SectionHeader('Money'),
        ListTile(
          leading: const Icon(Icons.currency_exchange_rounded),
          title: const Text('Currency'),
          subtitle: const Text(
            'New expenses in this group use it. Existing ones keep theirs.',
          ),
          trailing: Text(group.defaultCurrency),
          onTap: () async {
            final code = await pickCurrency(
              context,
              selected: group.defaultCurrency,
            );
            if (code != null) _patch(GroupInput(defaultCurrency: code));
          },
        ),
        SwitchListTile(
          secondary: const Icon(Icons.auto_fix_high_rounded),
          title: const Text('Simplify debts'),
          subtitle: const Text(
            'Combine debts so the group settles with fewer payments.',
          ),
          value: group.simplifyDebts,
          onChanged: (v) => _patch(GroupInput(simplifyDebts: v)),
        ),
        ListTile(
          leading: const Icon(Icons.download_rounded),
          title: const Text('Export as CSV'),
          onTap: _export,
        ),
        SectionHeader(
          'Members',
          trailing: TextButton.icon(
            onPressed: _addMember,
            icon: const Icon(Icons.person_add_alt_1_rounded),
            label: const Text('Add'),
          ),
        ),
        for (final m in group.members)
          ListTile(
            leading: UserAvatar(user: m),
            title: Text(
              m.id == meId ? '${m.displayName} (you)' : m.displayName,
            ),
            subtitle: Text(m.statusLine),
            trailing: m.id == meId
                ? null
                : IconButton(
                    tooltip: 'Remove',
                    icon: const Icon(Icons.person_remove_outlined),
                    onPressed: () => _removeMember(m.id, self: false),
                  ),
          ),
        InviteSection(group: group),
        const SectionHeader('Danger zone'),
        ListTile(
          leading: Icon(
            group.archived ? Icons.unarchive_outlined : Icons.archive_outlined,
          ),
          title: Text(group.archived ? 'Unarchive group' : 'Archive group'),
          onTap: () => _patch(
            GroupInput(archived: !group.archived),
            done: group.archived ? 'Group restored' : 'Group archived',
          ),
        ),
        if (meId != null)
          ListTile(
            leading: const Icon(Icons.logout_rounded),
            title: const Text('Leave group'),
            onTap: () => _removeMember(meId, self: true),
          ),
        ListTile(
          leading: Icon(Icons.delete_forever_rounded, color: scheme.error),
          title: Text('Delete group', style: TextStyle(color: scheme.error)),
          onTap: _delete,
        ),
      ],
    );
  }
}
