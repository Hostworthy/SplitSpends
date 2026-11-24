import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/features/admin/presentation/admin_screen.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/shell.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/account/presentation/account_screen.dart';
import 'package:splitspends/features/account/presentation/tokens_screen.dart';
import 'package:splitspends/features/categories/presentation/manage_categories_screen.dart';
import 'package:splitspends/features/activity/presentation/activity_screen.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';
import 'package:splitspends/features/auth/presentation/local_setup_screen.dart';
import 'package:splitspends/features/auth/presentation/login_screen.dart';
import 'package:splitspends/features/auth/presentation/otp_screen.dart';
import 'package:splitspends/features/auth/presentation/splash_screen.dart';
import 'package:splitspends/features/auth/presentation/welcome_screen.dart';
import 'package:splitspends/features/dashboard/presentation/dashboard_screen.dart';
import 'package:splitspends/features/expenses/presentation/detail/expense_detail_screen.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_editor_screen.dart';
import 'package:splitspends/features/expenses/presentation/settle/settle_screen.dart';
import 'package:splitspends/features/friends/presentation/friend_detail_screen.dart';
import 'package:splitspends/features/friends/presentation/friends_screen.dart';
import 'package:splitspends/features/groups/presentation/detail/group_detail_screen.dart';
import 'package:splitspends/features/groups/presentation/group_settings_screen.dart';
import 'package:splitspends/features/groups/presentation/groups_screen.dart';
import 'package:splitspends/features/invites/presentation/join_screen.dart';
import 'package:splitspends/features/invites/presentation/providers.dart';
import 'package:splitspends/features/receipts/presentation/scan_receipt_screen.dart';
import 'package:splitspends/features/sync/presentation/import_screen.dart';

final _rootKey = GlobalKey<NavigatorState>(debugLabel: 'root');

/// Width at which the app switches to rail + two panes.
const wideBreakpoint = 700.0;

bool isWide(BuildContext context) =>
    MediaQuery.sizeOf(context).width >= wideBreakpoint;

final routerProvider = Provider<GoRouter>((ref) {
  final refresh = ValueNotifier(0);
  ref.listen(sessionProvider, (_, _) => refresh.value++);
  ref.onDispose(refresh.dispose);

  return GoRouter(
    navigatorKey: _rootKey,
    initialLocation: Routes.home,
    refreshListenable: refresh,
    redirect: (context, state) {
      final session = ref.read(sessionProvider);
      final path = state.matchedLocation;
      if (!session.hasValue) {
        // Remember deep links while the session loads.
        if (path == Routes.splash) return null;
        return '${Routes.splash}?from=${Uri.encodeQueryComponent(state.uri.toString())}';
      }
      final onboarding =
          path.startsWith(Routes.welcome) || path.startsWith(Routes.login);
      // Invite links open for everyone; the screen explains what to do.
      final joining = path.startsWith('${Routes.joinPrefix}/');
      if (!session.requireValue.isReady) {
        return onboarding || joining ? null : Routes.welcome;
      }
      // On-device users may sign in to a server to sync (and import).
      if (path.startsWith(Routes.login) && session.requireValue.isLocal) {
        return null;
      }
      if (onboarding) {
        // Back to the invite that sent the user through sign-in.
        final pending = ref.read(pendingInviteProvider);
        return pending == null
            ? Routes.home
            : Routes.join(pending.token, server: pending.server);
      }
      if (path == Routes.splash) {
        final from = state.uri.queryParameters['from'];
        return (from == null || from.startsWith(Routes.splash))
            ? Routes.home
            : from;
      }
      return null;
    },
    routes: [
      GoRoute(
        path: Routes.splash,
        builder: (context, state) => const SplashScreen(),
      ),
      GoRoute(
        path: Routes.welcome,
        pageBuilder: (context, state) => _fade(state, const WelcomeScreen()),
        routes: [
          GoRoute(
            path: 'local',
            builder: (context, state) => const LocalSetupScreen(),
          ),
        ],
      ),
      GoRoute(
        path: Routes.login,
        pageBuilder: (context, state) => _fade(state, const LoginScreen()),
        routes: [
          GoRoute(
            path: 'import',
            builder: (context, state) => state.extra is LoginResult
                ? ImportScreen(login: state.extra! as LoginResult)
                : const LoginScreen(),
          ),
          GoRoute(
            path: 'verify',
            pageBuilder: (context, state) => _fade(
              state,
              OtpScreen(email: state.uri.queryParameters['email'] ?? ''),
            ),
          ),
        ],
      ),
      StatefulShellRoute.indexedStack(
        builder: (context, state, shell) =>
            AppShell(shell: shell, state: state),
        branches: [
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: Routes.home,
                builder: (context, state) => const DashboardScreen(),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: Routes.groups,
                builder: (context, state) => const GroupsScreen(),
                routes: [
                  GoRoute(
                    path: ':groupId',
                    pageBuilder: (context, state) {
                      final id = state.pathParameters['groupId']!;
                      return isWide(context)
                          ? NoTransitionPage(
                              key: state.pageKey,
                              child: GroupsScreen(selectedId: id),
                            )
                          : MaterialPage(
                              key: state.pageKey,
                              child: GroupDetailScreen(groupId: id),
                            );
                    },
                    routes: [
                      GoRoute(
                        path: 'settings',
                        builder: (context, state) => GroupSettingsScreen(
                          groupId: state.pathParameters['groupId']!,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: Routes.friends,
                builder: (context, state) => const FriendsScreen(),
                routes: [
                  GoRoute(
                    path: ':userId',
                    pageBuilder: (context, state) {
                      final id = state.pathParameters['userId']!;
                      return isWide(context)
                          ? NoTransitionPage(
                              key: state.pageKey,
                              child: FriendsScreen(selectedId: id),
                            )
                          : MaterialPage(
                              key: state.pageKey,
                              child: FriendDetailScreen(userId: id),
                            );
                    },
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: Routes.activity,
                builder: (context, state) => const ActivityScreen(),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: Routes.account,
                builder: (context, state) => const AccountScreen(),
                routes: [
                  GoRoute(
                    path: 'tokens',
                    builder: (context, state) => const TokensScreen(),
                  ),
                  GoRoute(
                    path: 'categories',
                    builder: (context, state) => const ManageCategoriesScreen(),
                  ),
                  GoRoute(
                    path: 'admin',
                    builder: (context, state) => const AdminScreen(),
                  ),
                ],
              ),
            ],
          ),
        ],
      ),
      GoRoute(
        parentNavigatorKey: _rootKey,
        path: Routes.newExpense,
        pageBuilder: (context, state) => MaterialPage(
          key: state.pageKey,
          fullscreenDialog: true,
          child: ExpenseEditorScreen(
            args: state.extra is ExpenseEditorArgs
                ? state.extra! as ExpenseEditorArgs
                : ExpenseEditorArgs(
                    groupId: state.uri.queryParameters['groupId'],
                    friendId: state.uri.queryParameters['friendId'],
                  ),
          ),
        ),
      ),
      GoRoute(
        parentNavigatorKey: _rootKey,
        path: '/expenses/:expenseId',
        builder: (context, state) =>
            ExpenseDetailScreen(expenseId: state.pathParameters['expenseId']!),
        routes: [
          GoRoute(
            parentNavigatorKey: _rootKey,
            path: 'edit',
            pageBuilder: (context, state) => MaterialPage(
              key: state.pageKey,
              fullscreenDialog: true,
              child: ExpenseEditorScreen(
                args: state.extra is ExpenseEditorArgs
                    ? state.extra! as ExpenseEditorArgs
                    : const ExpenseEditorArgs(),
                expenseId: state.pathParameters['expenseId'],
              ),
            ),
          ),
        ],
      ),
      GoRoute(
        parentNavigatorKey: _rootKey,
        path: Routes.settle,
        pageBuilder: (context, state) => MaterialPage(
          key: state.pageKey,
          fullscreenDialog: true,
          child: SettleScreen(
            args: state.extra is SettleArgs
                ? state.extra! as SettleArgs
                : SettleArgs(groupId: state.uri.queryParameters['groupId']),
          ),
        ),
      ),
      GoRoute(
        parentNavigatorKey: _rootKey,
        path: '${Routes.joinPrefix}/:token',
        builder: (context, state) => JoinScreen(
          token: state.pathParameters['token']!,
          server: state.uri.queryParameters['server'],
        ),
      ),
      GoRoute(
        parentNavigatorKey: _rootKey,
        path: Routes.scan,
        pageBuilder: (context, state) => MaterialPage(
          key: state.pageKey,
          fullscreenDialog: true,
          child: ScanReceiptScreen(
            groupId: state.uri.queryParameters['groupId'],
          ),
        ),
      ),
    ],
  );
});

CustomTransitionPage<void> _fade(GoRouterState state, Widget child) =>
    CustomTransitionPage(
      key: state.pageKey,
      child: child,
      transitionsBuilder: (context, animation, secondary, child) =>
          FadeTransition(opacity: animation, child: child),
    );
