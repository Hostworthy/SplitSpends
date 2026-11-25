import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/theme/theme.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/balance_text.dart';
import 'package:splitspends/features/groups/domain/group.dart';

/// Emoji badge on the group's color.
class GroupBadge extends StatelessWidget {
  const GroupBadge({super.key, required this.group, this.size = 48});

  final Group group;
  final double size;

  @override
  Widget build(BuildContext context) {
    final color = parseHexColor(group.color) ?? UserAvatar.colorFor(group.id);
    return Hero(
      tag: 'group-badge-${group.id}',
      child: Container(
        width: size,
        height: size,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.18),
          borderRadius: BorderRadius.circular(size * 0.3),
        ),
        child: Text(
          group.displayEmoji,
          style: TextStyle(fontSize: size * 0.48),
        ),
      ),
    );
  }
}

/// Compact card for horizontal lists (dashboard).
class GroupCard extends StatelessWidget {
  const GroupCard({super.key, required this.group});

  final Group group;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 180,
      child: Card(
        child: InkWell(
          onTap: () => context.go(Routes.group(group.id)),
          child: Padding(
            padding: const EdgeInsets.all(14),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    GroupBadge(group: group, size: 36),
                    const Spacer(),
                    AvatarStack(users: group.members, size: 20, max: 3),
                  ],
                ),
                const Spacer(),
                Text(
                  group.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.titleSmall,
                ),
                const SizedBox(height: 2),
                BalanceText(
                  balances: group.balances,
                  dense: true,
                  align: CrossAxisAlignment.start,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class NewGroupCard extends StatelessWidget {
  const NewGroupCard({super.key, required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      width: 140,
      child: Card(
        color: Colors.transparent,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppTheme.radius),
          side: BorderSide(color: scheme.outlineVariant, width: 1.5),
        ),
        child: InkWell(
          onTap: onTap,
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(Icons.add_rounded, color: scheme.primary, size: 32),
              const SizedBox(height: 6),
              Text('New group', style: TextStyle(color: scheme.primary)),
            ],
          ),
        ),
      ),
    );
  }
}

/// Row in the groups list.
class GroupListTile extends StatelessWidget {
  const GroupListTile({super.key, required this.group, this.selected = false});

  final Group group;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      selected: selected,
      selectedTileColor: Theme.of(context).colorScheme.secondaryContainer
          .withValues(alpha: 0.6),
      onTap: () => context.go(Routes.group(group.id)),
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      leading: GroupBadge(group: group),
      title: Text(group.name, maxLines: 1, overflow: TextOverflow.ellipsis),
      subtitle: Text(
        '${group.members.length} ${group.members.length == 1 ? 'member' : 'members'}'
        '${group.archived ? ' · archived' : ''}',
      ),
      trailing: BalanceText(balances: group.balances),
    );
  }
}
