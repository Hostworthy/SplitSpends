import 'dart:convert';
import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/widgets/swatches.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/download.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/person_dialog.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/default_split_section.dart';
import 'package:splitspends/features/groups/presentation/emoji_picker.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';
import 'package:splitspends/features/invites/presentation/invite_section.dart';

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

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  void _rename(String name) =>
      _patch(context, group, GroupInput(name: name.trim()), done: 'Renamed');

  @override
  Widget build(BuildContext context) {
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
                onPressed: () => _rename(_name.text),
              ),
            ),
            onSubmitted: _rename,
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
                  onSelected: (_) =>
                      _patch(context, group, GroupInput(kind: kind)),
                ),
            ],
          ),
        ),
        _LookSection(group: group),
        _MoneySection(group: group),
        _MembersSection(group: group),
        InviteSection(group: group),
        DefaultSplitSection(group: group),
        _DangerZone(group: group),
      ],
    );
  }
}

Future<void> _patch(
  BuildContext context,
  Group group,
  GroupInput input, {
  String? done,
}) => changeLedger(
  context,
  (container) =>
      container.read(groupsRepositoryProvider).update(group.id, input),
  done: done,
);

Future<void> _removeMember(
  BuildContext context,
  Group group,
  String userId, {
  required bool self,
}) async {
  final ok = await confirm(
    context,
    title: self ? 'Leave group?' : 'Remove member?',
    message: 'Only possible when their balance in this group is settled.',
    action: self ? 'Leave' : 'Remove',
  );
  if (!ok || !context.mounted) return;
  final removed = await changeLedger(
    context,
    (container) =>
        container.read(groupsRepositoryProvider).removeMember(group.id, userId),
  );
  if (removed && self && context.mounted) context.go(Routes.groups);
}

class _LookSection extends StatelessWidget {
  const _LookSection({required this.group});

  final Group group;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Look'),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              for (final e in {
                if (!quickEmojis.contains(group.displayEmoji))
                  group.displayEmoji,
                ...quickEmojis,
              })
                ChoiceChip(
                  label: Text(e, style: const TextStyle(fontSize: 18)),
                  selected: group.displayEmoji == e,
                  onSelected: (_) =>
                      _patch(context, group, GroupInput(emoji: e)),
                ),
              ActionChip(
                key: const Key('emoji-more'),
                avatar: const Icon(Icons.add_reaction_outlined, size: 18),
                label: const Text('More'),
                onPressed: () async {
                  final e = await pickEmoji(
                    context,
                    selected: group.displayEmoji,
                  );
                  if (e != null && context.mounted) {
                    unawaited(_patch(context, group, GroupInput(emoji: e)));
                  }
                },
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
          child: Swatches(
            selected: group.color,
            onSelected: (hex) => _patch(context, group, GroupInput(color: hex)),
          ),
        ),
      ],
    );
  }
}

class _MoneySection extends StatelessWidget {
  const _MoneySection({required this.group});

  final Group group;

  Future<void> _export(BuildContext context) async {
    try {
      final bytes = await ProviderScope.containerOf(
        context,
        listen: false,
      ).read(groupsRepositoryProvider).export(group.id);
      final name = '${group.name.replaceAll(RegExp(r'[^\w\- ]'), '')}.csv';
      if (!saveDownload(name, bytes, 'text/csv')) {
        await Clipboard.setData(ClipboardData(text: utf8.decode(bytes)));
        if (context.mounted) {
          showMessage(context, 'CSV copied to the clipboard');
        }
      }
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
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
            if (code != null && context.mounted) {
              unawaited(
                _patch(context, group, GroupInput(defaultCurrency: code)),
              );
            }
          },
        ),
        SwitchListTile(
          secondary: const Icon(Icons.auto_fix_high_rounded),
          title: const Text('Simplify debts'),
          subtitle: const Text(
            'Combine debts so the group settles with fewer payments.',
          ),
          value: group.simplifyDebts,
          onChanged: (v) =>
              _patch(context, group, GroupInput(simplifyDebts: v)),
        ),
        ListTile(
          leading: const Icon(Icons.download_rounded),
          title: const Text('Export as CSV'),
          onTap: () => _export(context),
        ),
      ],
    );
  }
}

class _MembersSection extends ConsumerWidget {
  const _MembersSection({required this.group});

  final Group group;

  Future<void> _add(BuildContext context) async {
    final entry = await showPersonDialog(context, title: 'Add member');
    if (entry == null || !context.mounted) return;
    await changeLedger(
      context,
      (container) => container
          .read(groupsRepositoryProvider)
          .addMember(group.id, email: entry.email, name: entry.name),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final meId = ref.watch(meProvider)?.id;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SectionHeader(
          'Members',
          trailing: TextButton.icon(
            onPressed: () => _add(context),
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
                    onPressed: () =>
                        _removeMember(context, group, m.id, self: false),
                  ),
          ),
      ],
    );
  }
}

class _DangerZone extends ConsumerWidget {
  const _DangerZone({required this.group});

  final Group group;

  Future<void> _delete(BuildContext context) async {
    final ok = await confirm(
      context,
      title: 'Delete group?',
      message:
          'This removes ${group.name} and its expenses for everyone. Only '
          'possible when everyone is settled up.',
      action: 'Delete',
    );
    if (!ok || !context.mounted) return;
    final deleted = await changeLedger(
      context,
      (container) => container.read(groupsRepositoryProvider).delete(group.id),
    );
    if (deleted && context.mounted) context.go(Routes.groups);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final meId = ref.watch(meProvider)?.id;
    final scheme = Theme.of(context).colorScheme;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Danger zone'),
        ListTile(
          leading: Icon(
            group.archived ? Icons.unarchive_outlined : Icons.archive_outlined,
          ),
          title: Text(group.archived ? 'Unarchive group' : 'Archive group'),
          onTap: () => _patch(
            context,
            group,
            GroupInput(archived: !group.archived),
            done: group.archived ? 'Group restored' : 'Group archived',
          ),
        ),
        // On-device groups have nobody else who could open them.
        if (meId != null && ref.watch(appModeProvider) != AppMode.local)
          ListTile(
            leading: const Icon(Icons.logout_rounded),
            title: const Text('Leave group'),
            onTap: () => _removeMember(context, group, meId, self: true),
          ),
        ListTile(
          leading: Icon(Icons.delete_forever_rounded, color: scheme.error),
          title: Text('Delete group', style: TextStyle(color: scheme.error)),
          onTap: () => _delete(context),
        ),
      ],
    );
  }
}
