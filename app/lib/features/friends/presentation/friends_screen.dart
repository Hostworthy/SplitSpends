import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/router.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/balance_text.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/friends/presentation/add_friend_dialog.dart';
import 'package:splitspends/features/friends/presentation/friend_detail_screen.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';

/// Friends list; on wide screens also shows the selected friend.
class FriendsScreen extends ConsumerWidget {
  const FriendsScreen({super.key, this.selectedId});

  final String? selectedId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final list = _FriendsList(selectedId: selectedId);
    if (!isWide(context)) return list;
    return Row(
      children: [
        SizedBox(width: 380, child: list),
        const VerticalDivider(width: 1),
        Expanded(
          child: selectedId == null
              ? const EmptyState(
                  icon: Icons.person_rounded,
                  title: 'Pick a friend',
                  message: 'See what you owe each other across all groups.',
                )
              : FriendDetailScreen(
                  key: ValueKey(selectedId),
                  userId: selectedId!,
                  embedded: true,
                ),
        ),
      ],
    );
  }
}

class _FriendsList extends ConsumerStatefulWidget {
  const _FriendsList({this.selectedId});

  final String? selectedId;

  @override
  ConsumerState<_FriendsList> createState() => _FriendsListState();
}

class _FriendsListState extends ConsumerState<_FriendsList> {
  String _filter = '';

  @override
  Widget build(BuildContext context) {
    final friends = ref.watch(friendsProvider);
    return Scaffold(
      appBar: AppBar(
        automaticallyImplyLeading: false,
        title: const Text('Friends'),
        actions: [
          IconButton(
            tooltip: 'Add friend',
            icon: const Icon(Icons.person_add_alt_1_rounded),
            onPressed: () => showAddFriendDialog(context),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          ref.invalidate(friendsProvider);
          await ref.read(friendsProvider.future);
        },
        child: AsyncView(
          value: friends,
          onRetry: () => ref.invalidate(friendsProvider),
          data: (items) {
            if (items.isEmpty) {
              return ListView(
                children: [
                  const SizedBox(height: 60),
                  EmptyState(
                    icon: Icons.people_alt_rounded,
                    title: 'No friends yet',
                    message: 'Add friends by email to split expenses outside of groups.',
                    action: FilledButton.icon(
                      onPressed: () => showAddFriendDialog(context),
                      icon: const Icon(Icons.person_add_alt_1_rounded),
                      label: const Text('Add friend'),
                    ),
                  ),
                ],
              );
            }
            final q = _filter.toLowerCase();
            final shown = items
                .where(
                  (f) =>
                      q.isEmpty ||
                      f.user.displayName.toLowerCase().contains(q) ||
                      f.user.email.toLowerCase().contains(q),
                )
                .toList();
            return ListView(
              padding: const EdgeInsets.only(bottom: 96),
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
                  child: TextField(
                    onChanged: (v) => setState(() => _filter = v),
                    decoration: const InputDecoration(
                      isDense: true,
                      prefixIcon: Icon(Icons.search_rounded),
                      hintText: 'Search friends',
                    ),
                  ),
                ),
                for (final f in shown)
                  ListTile(
                    selected: f.user.id == widget.selectedId,
                    selectedTileColor: Theme.of(context)
                        .colorScheme
                        .secondaryContainer
                        .withValues(alpha: 0.6),
                    onTap: () => context.go(Routes.friend(f.user.id)),
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 4,
                    ),
                    leading: Hero(
                      tag: 'friend-${f.user.id}',
                      child: UserAvatar(user: f.user, size: 44),
                    ),
                    title: Text(f.user.displayName),
                    subtitle: f.user.isInvited
                        ? const Text('Invited')
                        : Text(f.user.email),
                    trailing: BalanceText(
                      balances: f.balances,
                      owedLabel: 'owes you',
                      oweLabel: 'you owe',
                    ),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }
}
