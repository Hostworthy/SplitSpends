import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
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

class ActivityTile extends StatelessWidget {
  const ActivityTile({super.key, required this.activity});

  final Activity activity;

  void _open(BuildContext context) {
    if (activity.expenseId != null && !activity.kind.endsWith('deleted')) {
      context.push(Routes.expense(activity.expenseId!));
    } else if (activity.groupId != null && !activity.kind.endsWith('removed')) {
      context.go(Routes.group(activity.groupId!));
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return ListTile(
      onTap: () => _open(context),
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
      title: Text(activity.summary, maxLines: 3),
      subtitle: Text(
        Dates.relative(activity.time),
        style: TextStyle(color: scheme.onSurfaceVariant),
      ),
    );
  }
}
