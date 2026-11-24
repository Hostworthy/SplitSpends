import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/features/activity/domain/activity.dart';

IconData _kindIcon(String kind) {
  if (kind.startsWith('payment')) return Icons.payments_rounded;
  if (kind.startsWith('comment')) return Icons.chat_bubble_rounded;
  if (kind.endsWith('deleted') || kind.endsWith('removed')) {
    return Icons.delete_rounded;
  }
  if (kind.startsWith('group')) return Icons.groups_rounded;
  if (kind.startsWith('friend')) return Icons.person_add_rounded;
  if (kind.endsWith('updated')) return Icons.edit_rounded;
  return Icons.receipt_long_rounded;
}

/// Server summaries name the actor ("Ada added ..."); show "You" for your
/// own actions so the feed reads the same on the device and on a server.
String personalSummary(Activity activity, String? meId) {
  final name = activity.actor.name;
  if (meId == null || activity.actor.id != meId || name.isEmpty) {
    return activity.summary;
  }
  if (activity.summary.startsWith('$name ')) {
    return 'You${activity.summary.substring(name.length)}';
  }
  return activity.summary;
}

class ActivityTile extends ConsumerWidget {
  const ActivityTile({super.key, required this.activity, this.linkable = true});

  final Activity activity;

  /// False inside the expense's own page.
  final bool linkable;

  void _open(BuildContext context) {
    if (activity.expenseId != null && !activity.kind.endsWith('deleted')) {
      context.push(Routes.expense(activity.expenseId!));
    } else if (activity.groupId != null && !activity.kind.endsWith('removed')) {
      context.go(Routes.group(activity.groupId!));
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final scheme = Theme.of(context).colorScheme;
    final summary = personalSummary(activity, ref.watch(meProvider)?.id);
    return ListTile(
      onTap: linkable ? () => _open(context) : null,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      leading: Stack(
        clipBehavior: Clip.none,
        children: [
          UserAvatar(user: activity.actor, size: 42),
          Positioned(
            right: -4,
            bottom: -4,
            child: Container(
              padding: const EdgeInsets.all(3),
              decoration: BoxDecoration(
                color: scheme.secondaryContainer,
                shape: BoxShape.circle,
                border: Border.all(color: scheme.surface, width: 2),
              ),
              child: Icon(
                _kindIcon(activity.kind),
                size: 12,
                color: scheme.onSecondaryContainer,
              ),
            ),
          ),
        ],
      ),
      title: Text(summary, maxLines: 3),
      subtitle: Text(
        Dates.relative(activity.time),
        style: TextStyle(color: scheme.onSurfaceVariant),
      ),
    );
  }
}
