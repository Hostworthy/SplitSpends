import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/friends/presentation/add_friend_dialog.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/group_card.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

/// "With you and: …" — choose a group, or friends for a non-group expense.
class WithSection extends ConsumerWidget {
  const WithSection({
    super.key,
    required this.form,
    required this.people,
    required this.onGroupMembers,
  });

  final ExpenseForm form;
  final Map<String, User> people;
  final ValueChanged<List<User>> onGroupMembers;

  Future<void> _pickGroup(BuildContext context, WidgetRef ref) async {
    final groups = await ref.read(groupsProvider.future);
    if (!context.mounted) return;
    final picked = await showModalBottomSheet<Object>(
      useRootNavigator: true,
      context: context,
      isScrollControlled: true,
      builder: (context) => SafeArea(
        child: ListView(
          shrinkWrap: true,
          children: [
            ListTile(
              leading: const Icon(Icons.person_rounded),
              title: const Text('No group (just friends)'),
              selected: form.groupId == null,
              onTap: () => Navigator.pop(context, false),
            ),
            for (final g in groups)
              ListTile(
                leading: GroupBadge(group: g, size: 36),
                title: Text(g.name),
                selected: g.id == form.groupId,
                onTap: () => Navigator.pop(context, g),
              ),
          ],
        ),
      ),
    );
    if (!context.mounted) return;
    if (picked is Group) {
      onGroupMembers(picked.members);
      form.setPeople(
        picked.members.map((m) => m.id).toList(),
        group: picked.id,
      );
      _setCurrency(context, ref, picked.defaultCurrency);
      final preset = picked.defaultSplit;
      if (preset != null) form.applyDefault(preset.method, preset.values);
    } else if (picked == false && form.groupId != null) {
      form.setPeople([form.meId]);
      final mine = ref.read(meProvider)?.defaultCurrency;
      if (mine != null) _setCurrency(context, ref, mine);
    }
  }

  void _setCurrency(BuildContext context, WidgetRef ref, String code) {
    if (code == form.currency) return;
    final config = ref.read(configValueProvider);
    final cleared = form.setCurrency(
      code,
      from: config.currency(form.currency).exponent,
      to: config.currency(code).exponent,
    );
    if (!cleared) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('Amounts are now in $code. Please enter them again.'),
      ),
    );
  }

  Future<void> _pickFriends(BuildContext context, WidgetRef ref) async {
    final friends = await ref.read(friendsProvider.future);
    if (!context.mounted) return;
    final selected = {...form.people}..remove(form.meId);
    final result = await showModalBottomSheet<Set<String>>(
      useRootNavigator: true,
      context: context,
      isScrollControlled: true,
      builder: (context) => StatefulBuilder(
        builder: (context, setState) => SafeArea(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Flexible(
                child: ListView(
                  shrinkWrap: true,
                  children: [
                    for (final f in friends)
                      CheckboxListTile(
                        value: selected.contains(f.user.id),
                        secondary: UserAvatar(user: f.user),
                        title: Text(f.user.displayName),
                        subtitle: Text(f.user.statusLine),
                        onChanged: (v) => setState(() {
                          v ?? false
                              ? selected.add(f.user.id)
                              : selected.remove(f.user.id);
                        }),
                      ),
                    ListTile(
                      leading: const Icon(Icons.person_add_alt_1_rounded),
                      title: const Text('Add a new friend'),
                      onTap: () {
                        Navigator.pop(context);
                        showAddFriendDialog(context);
                      },
                    ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(16),
                child: SizedBox(
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: () => Navigator.pop(context, selected),
                    child: const Text('Done'),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
    if (result != null) form.setPeople([form.meId, ...result]);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final groups = ref.watch(groupsProvider).value ?? const <Group>[];
    final groupId = form.groupId;
    // The editor loads its group itself; the list may not have it yet.
    final group =
        groups.where((g) => g.id == groupId).firstOrNull ??
        (groupId == null ? null : ref.watch(groupProvider(groupId)).value);
    final text = Theme.of(context).textTheme;
    final others = form.people.where((id) => id != form.meId).toList();
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Text('With you and', style: text.bodyMedium),
          ActionChip(
            avatar: Icon(
              form.groupId == null
                  ? Icons.person_rounded
                  : Icons.groups_rounded,
              size: 18,
            ),
            label: Text(
              form.groupId == null ? 'No group' : group?.name ?? 'Group',
            ),
            tooltip: form.groupLocked
                ? 'The group of an expense cannot change'
                : null,
            onPressed: form.groupLocked ? null : () => _pickGroup(context, ref),
          ),
          if (form.groupId == null) ...[
            for (final id in others)
              InputChip(
                avatar: people[id] == null
                    ? null
                    : UserAvatar(user: people[id]!, size: 24),
                label: Text(people[id]?.firstName ?? 'Friend'),
                onDeleted: () =>
                    form.setPeople(form.people.where((p) => p != id).toList()),
              ),
            ActionChip(
              avatar: const Icon(Icons.add_rounded, size: 18),
              label: Text(others.isEmpty ? 'Choose friends' : 'Add'),
              onPressed: () => _pickFriends(context, ref),
            ),
          ],
        ],
      ),
    );
  }
}
