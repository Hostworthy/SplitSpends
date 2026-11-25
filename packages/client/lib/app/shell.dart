import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/push/presentation/providers.dart';
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
    ref.watch(pushSyncProvider);
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
          Flexible(
            child: Text(
              'Split Spends',
              overflow: TextOverflow.ellipsis,
              style: Theme.of(context).textTheme.titleMedium?.copyWith(
                fontWeight: FontWeight.w800,
                color: scheme.onSurface,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// The app mark, drawn like the app icon (packages/brand/icon.svg): a white
/// and a mint coin overlapping on a teal rounded square.
class AppLogo extends StatelessWidget {
  const AppLogo({super.key, this.size = 48});

  final double size;

  @override
  Widget build(BuildContext context) => Semantics(
    label: 'Split Spends',
    image: true,
    child: CustomPaint(size: Size.square(size), painter: const _LogoPainter()),
  );
}

class _LogoPainter extends CustomPainter {
  const _LogoPainter();

  // Geometry of the 1024-unit icon in packages/brand/icon.svg.
  static const _left = Offset(392 / 1024, 0.5);
  static const _right = Offset(632 / 1024, 0.5);
  static const _radius = 232 / 1024;
  static const _cut = 272 / 1024;
  static const _ring = 167 / 1024;
  static const _stroke = 16 / 1024;

  @override
  void paint(Canvas canvas, Size size) {
    final s = size.width;
    final square = RRect.fromRectAndRadius(
      Offset.zero & size,
      Radius.circular(s * 0.225),
    );
    canvas.drawRRect(
      square,
      Paint()
        ..shader = const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF12907A), Color(0xFF0A5E4E)],
        ).createShader(Offset.zero & size),
    );
    Path circle(Offset c, double r) =>
        Path()..addOval(Rect.fromCircle(center: c * s, radius: r * s));
    final left = Path.combine(
      PathOperation.difference,
      circle(_left, _radius),
      circle(_right, _cut),
    );
    canvas.drawPath(left, Paint()..color = const Color(0xFFFFFFFF));
    canvas.drawCircle(
      _right * s,
      _radius * s,
      Paint()..color = const Color(0xFF9DEBCF),
    );
    if (s >= 28) {
      final ring = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = _stroke * s;
      canvas
        ..save()
        ..clipPath(left)
        ..drawCircle(
          _left * s,
          _ring * s,
          ring..color = const Color(0xFFD6EFE6),
        )
        ..restore()
        ..drawCircle(
          _right * s,
          _ring * s,
          ring..color = const Color(0xFF6FD3B2),
        );
    }
  }

  @override
  bool shouldRepaint(_LogoPainter oldDelegate) => false;
}
