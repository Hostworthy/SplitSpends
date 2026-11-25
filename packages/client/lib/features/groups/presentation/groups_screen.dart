import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/detail/group_detail_screen.dart';
import 'package:splitspends/features/groups/presentation/group_card.dart';
import 'package:splitspends/features/groups/presentation/group_form_sheet.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';
import 'package:splitspends/features/invites/presentation/join_link_dialog.dart';

/// Groups list; on wide screens also shows the selected group beside it.
class GroupsScreen extends ConsumerWidget {
  const GroupsScreen({super.key, this.selectedId});

  final String? selectedId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final list = _GroupsList(selectedId: selectedId);
    if (!isWide(context)) return list;
    return Row(
      children: [
        SizedBox(width: 380, child: list),
        const VerticalDivider(width: 1),
        Expanded(
          child: selectedId == null
              ? const EmptyState(
                  icon: Icons.groups_rounded,
                  title: 'Pick a group',
                  message: 'Select a group to see its expenses and balances.',
                )
              // The list beside this pane already shows the group's badge as a
              // hero, and two heroes with one tag break route changes.
              : HeroMode(
                  enabled: false,
                  child: GroupDetailScreen(
                    key: ValueKey(selectedId),
                    groupId: selectedId!,
                    embedded: true,
                  ),
                ),
        ),
      ],
    );
  }
}

class _GroupsList extends ConsumerStatefulWidget {
  const _GroupsList({this.selectedId});

  final String? selectedId;

  @override
  ConsumerState<_GroupsList> createState() => _GroupsListState();
}

class _GroupsListState extends ConsumerState<_GroupsList> {
  bool _showArchived = false;

  @override
  Widget build(BuildContext context) {
    final groups = ref.watch(groupsProvider);
    return Scaffold(
      appBar: AppBar(
        automaticallyImplyLeading: false,
        title: const Text('Groups'),
        actions: [
          IconButton(
            key: const Key('join-with-link'),
            tooltip: 'Join with a link',
            icon: const Icon(Icons.link_rounded),
            onPressed: () => showJoinLinkDialog(context),
          ),
          IconButton(
            tooltip: 'New group',
            icon: const Icon(Icons.group_add_rounded),
            onPressed: () => showNewGroupSheet(context),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          ref.invalidate(groupsProvider);
          ref.invalidate(archivedGroupsProvider);
          await ref.read(groupsProvider.future);
        },
        child: AsyncView(
          value: groups,
          onRetry: () => ref.invalidate(groupsProvider),
          data: (items) => items.isEmpty && !_showArchived
              ? ListView(
                  children: [
                    const SizedBox(height: 60),
                    EmptyState(
                      icon: Icons.groups_rounded,
                      title: 'No groups yet',
                      message: 'Create a group for your home, a trip, or anything you share.',
                      action: Wrap(
                        spacing: 8,
                        runSpacing: 8,
                        alignment: WrapAlignment.center,
                        children: [
                          FilledButton.icon(
                            onPressed: () => showNewGroupSheet(context),
                            icon: const Icon(Icons.add_rounded),
                            label: const Text('New group'),
                          ),
                          OutlinedButton.icon(
                            onPressed: () => showJoinLinkDialog(context),
                            icon: const Icon(Icons.link_rounded),
                            label: const Text('Join with a link'),
                          ),
                        ],
                      ),
                    ),
                    _archivedToggle(),
                  ],
                )
              : ListView(
                  padding: const EdgeInsets.only(bottom: 96),
                  children: [
                    for (final g in items)
                      GroupListTile(
                        group: g,
                        selected: g.id == widget.selectedId,
                      ),
                    _archivedToggle(),
                    if (_showArchived) ..._archived(),
                  ],
                ),
        ),
      ),
    );
  }

  Widget _archivedToggle() => Center(
    child: TextButton.icon(
      onPressed: () => setState(() => _showArchived = !_showArchived),
      icon: Icon(
        _showArchived ? Icons.expand_less_rounded : Icons.inventory_2_outlined,
      ),
      label: Text(_showArchived ? 'Hide archived' : 'Show archived groups'),
    ),
  );

  List<Widget> _archived() {
    final archived = ref.watch(archivedGroupsProvider);
    return switch (archived) {
      AsyncValue<List<Group>>(:final value?) when value.isEmpty => const [
        Padding(
          padding: EdgeInsets.all(16),
          child: Center(child: Text('No archived groups')),
        ),
      ],
      AsyncValue<List<Group>>(:final value?) => [
        for (final g in value)
          GroupListTile(group: g, selected: g.id == widget.selectedId),
      ],
      AsyncValue(:final error?) => [
        InlineError(
          key: const Key('archived-error'),
          error: error,
          onRetry: () => ref.invalidate(archivedGroupsProvider),
        ),
      ],
      _ => const [LinearProgressIndicator()],
    };
  }
}
