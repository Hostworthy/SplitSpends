import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/sync_bar.dart';

class _Destination {
  const _Destination(this.label, this.icon, this.selectedIcon);

  final String label;
  final IconData icon;
  final IconData selectedIcon;
}

const _destinations = [
  _Destination('Home', Icons.space_dashboard_outlined, Icons.space_dashboard),
  _Destination('Groups', Icons.groups_outlined, Icons.groups),
  _Destination('Friends', Icons.person_outline_rounded, Icons.person_rounded),
  _Destination('Activity', Icons.bolt_outlined, Icons.bolt),
  _Destination('Account', Icons.settings_outlined, Icons.settings),
];

/// Adaptive app frame: bottom navigation on phones, a rail on wide screens.
class AppShell extends ConsumerWidget {
  const AppShell({super.key, required this.shell, required this.state});

  final StatefulNavigationShell shell;
  final GoRouterState state;

  void _go(int index) {
    HapticFeedback.selectionClick();
    shell.goBranch(index, initialLocation: index == shell.currentIndex);
  }

  void _addExpense(BuildContext context) {
    HapticFeedback.lightImpact();
    final groupId = state.pathParameters['groupId'];
    final friendId = state.pathParameters['userId'];
    context.push(
      Routes.newExpense,
      extra: ExpenseEditorArgs(groupId: groupId, friendId: friendId),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.listen(localMaintenanceProvider, (_, next) {
      if ((next.value ?? 0) > 0) ref.refreshLedger();
    });
    ref.listen(outboxSyncProvider, (was, now) {
      if (was?.round != now.round) ref.refreshLedger();
    });
    final body = Column(
      children: [
        const SyncBar(),
        Expanded(child: shell),
      ],
    );
    final showFab = shell.currentIndex <= 2;
    if (!isWide(context)) {
      return Scaffold(
        body: body,
        floatingActionButton: AnimatedScale(
          scale: showFab ? 1 : 0,
          duration: const Duration(milliseconds: 180),
          child: FloatingActionButton.extended(
            heroTag: 'add-expense',
            onPressed: showFab ? () => _addExpense(context) : null,
            icon: const Icon(Icons.add_rounded),
            label: const Text('Add expense'),
          ),
        ),
        bottomNavigationBar: NavigationBar(
          selectedIndex: shell.currentIndex,
          onDestinationSelected: _go,
          destinations: [
            for (final d in _destinations)
              NavigationDestination(
                icon: Icon(d.icon),
                selectedIcon: Icon(d.selectedIcon),
                label: d.label,
              ),
          ],
        ),
      );
    }

    final extended = MediaQuery.sizeOf(context).width >= 1180;
    final me = ref.watch(meProvider);
    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            extended: extended,
            minExtendedWidth: 220,
            selectedIndex: shell.currentIndex,
            onDestinationSelected: _go,
            labelType: extended
                ? NavigationRailLabelType.none
                : NavigationRailLabelType.all,
            leading: Padding(
              padding: const EdgeInsets.only(top: 8, bottom: 16),
              child: Column(
                children: [
                  if (extended) const _Brand(),
                  const SizedBox(height: 12),
                  extended
                      ? FloatingActionButton.extended(
                          heroTag: 'add-expense',
                          elevation: 0,
                          onPressed: () => _addExpense(context),
                          icon: const Icon(Icons.add_rounded),
                          label: const Text('Add expense'),
                        )
                      : FloatingActionButton(
                          heroTag: 'add-expense',
                          elevation: 0,
                          tooltip: 'Add expense',
                          onPressed: () => _addExpense(context),
                          child: const Icon(Icons.add_rounded),
                        ),
                ],
              ),
            ),
            trailing: me == null
                ? null
                : Expanded(
                    child: Align(
                      alignment: Alignment.bottomCenter,
                      child: Padding(
                        padding: const EdgeInsets.only(bottom: 20),
                        child: Tooltip(
                          message: me.email ?? me.displayName,
                          child: UserAvatar(user: me, size: 36),
                        ),
                      ),
                    ),
                  ),
            destinations: [
              for (final d in _destinations)
                NavigationRailDestination(
                  icon: Icon(d.icon),
                  selectedIcon: Icon(d.selectedIcon),
                  label: Text(d.label),
                ),
            ],
          ),
          const VerticalDivider(width: 1),
          Expanded(child: body),
        ],
      ),
    );
  }
}

class _Brand extends StatelessWidget {
  const _Brand();

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      width: 188,
      child: Row(
        children: [
          const AppLogo(size: 32),
          const SizedBox(width: 10),
          Text(
            'SplitSpends',
            style: Theme.of(context).textTheme.titleMedium?.copyWith(
              fontWeight: FontWeight.w800,
              color: scheme.onSurface,
            ),
          ),
        ],
      ),
    );
  }
}

/// The app mark: two overlapping coins.
class AppLogo extends StatelessWidget {
  const AppLogo({super.key, this.size = 48});

  final double size;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      width: size * 1.35,
      height: size,
      child: Stack(
        children: [
          Positioned(
            left: 0,
            child: _Coin(size: size, color: scheme.primary),
          ),
          Positioned(
            right: 0,
            child: _Coin(
              size: size,
              color: scheme.tertiary.withValues(alpha: 0.9),
            ),
          ),
        ],
      ),
    );
  }
}

class _Coin extends StatelessWidget {
  const _Coin({required this.size, required this.color});

  final double size;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
    width: size,
    height: size,
    decoration: BoxDecoration(
      color: color,
      shape: BoxShape.circle,
      border: Border.all(
        color: Theme.of(context).colorScheme.surface,
        width: size * 0.06,
      ),
    ),
  );
}
