import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/invites/domain/invite.dart';
import 'package:splitspends/features/invites/presentation/join_screen.dart';
import 'package:splitspends/features/invites/presentation/providers.dart';

const _token = 'k3V9xQ2mN8pL4rT7wY1zA6bC0dE5fG8hJ2kM4nP6qR8';
const _server = 'https://split.example.com';

final _group = Group.fromJson({
  'id': 'g1',
  'name': 'Flat',
  'kind': 'home',
  'defaultCurrency': 'EUR',
  'simplifyDebts': true,
  'archived': false,
  'members': [
    {'id': 'ada', 'name': 'Ada'},
  ],
  'balances': <Object>[],
});

const _connected = Session(
  mode: AppMode.connected,
  serverUrl: _server,
  token: 'tok',
  me: Me(id: 'cy', name: 'Cy'),
);

class _Session extends SessionController {
  _Session(this.session);

  final Session session;

  @override
  Future<Session> build() async => session;
}

class _Previews implements InvitePreviews {
  @override
  Future<InvitePreview> preview(String serverUrl, String token) async =>
      const InvitePreview(groupName: 'Flat', memberCount: 1, invitedBy: 'Ada');
}

/// Holds every accept until the test answers it.
class _Invites extends Fake implements InvitesRepository {
  final accepts = <String>[];
  final pending = <Completer<Group>>[];

  @override
  Future<Group> accept(String token) {
    accepts.add(token);
    final done = Completer<Group>();
    pending.add(done);
    return done.future;
  }
}

typedef _Harness = ({
  ProviderContainer container,
  GoRouter router,
  _Invites invites,
});

/// Opens the link for [_token] on [_server] over a home page, with
/// [pending] as the invite the person opened before signing in.
Future<_Harness> _open(
  WidgetTester tester, {
  Session session = _connected,
  JoinLink? pending,
}) async {
  tester.view.physicalSize = const Size(420, 900);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  final invites = _Invites();
  final router = GoRouter(
    routes: [
      GoRoute(path: '/', builder: (_, _) => const Text('Home')),
      GoRoute(
        path: '/join/:token',
        builder: (_, state) => JoinScreen(
          token: state.pathParameters['token']!,
          server: state.uri.queryParameters['server'],
        ),
      ),
      GoRoute(
        path: '/groups/:id',
        builder: (_, state) => Text('Group ${state.pathParameters['id']}'),
      ),
    ],
  );
  addTearDown(router.dispose);
  final container = ProviderContainer(
    retry: (_, _) => null,
    overrides: [
      sessionProvider.overrideWith(() => _Session(session)),
      invitePreviewsProvider.overrideWithValue(_Previews()),
      invitesRepositoryProvider.overrideWithValue(invites),
    ],
  );
  addTearDown(container.dispose);
  container.read(pendingInviteProvider.notifier).set(pending);
  await tester.pumpWidget(
    UncontrolledProviderScope(
      container: container,
      child: MaterialApp.router(routerConfig: router),
    ),
  );
  await tester.pumpAndSettle();
  unawaited(
    router.push('/join/$_token?server=${Uri.encodeQueryComponent(_server)}'),
  );
  // Not pumpAndSettle: an auto-join leaves a spinner turning until the
  // test answers the accept.
  for (var i = 0; i < 10; i++) {
    await tester.pump(const Duration(milliseconds: 100));
  }
  return (container: container, router: router, invites: invites);
}

void main() {
  testWidgets('join auto-accepts only the pending token', (tester) async {
    final back = await _open(tester, pending: const JoinLink(token: _token));
    expect(back.invites.accepts, [_token]);
    back.invites.pending.single.complete(_group);
    await tester.pumpAndSettle();
    expect(find.text('Group g1'), findsOneWidget);
    expect(back.container.read(pendingInviteProvider), isNull);
  });

  testWidgets('another pending link is dropped, not joined', (tester) async {
    final other = await _open(tester, pending: const JoinLink(token: 'older'));
    expect(other.invites.accepts, isEmpty);
    expect(other.container.read(pendingInviteProvider), isNull);
    expect(find.byKey(const Key('join-accept')), findsOneWidget);
  });

  testWidgets('a link opened directly waits for Join group', (tester) async {
    final direct = await _open(tester);
    expect(direct.invites.accepts, isEmpty);
    await tester.tap(find.byKey(const Key('join-accept')));
    await tester.pump();
    expect(direct.invites.accepts, [_token]);
  });

  testWidgets('leaving during an accept still clears the pending invite', (
    tester,
  ) async {
    final left = await _open(tester, pending: const JoinLink(token: _token));
    expect(left.invites.accepts, [_token]);
    left.router.pop();
    await tester.pumpAndSettle();
    expect(find.byType(JoinScreen), findsNothing);

    left.invites.pending.single.complete(_group);
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(left.container.read(pendingInviteProvider), isNull);
  });

  testWidgets('a failed accept shows why and offers Join group again', (
    tester,
  ) async {
    final failed = await _open(tester);
    await tester.tap(find.byKey(const Key('join-accept')));
    await tester.pump();
    failed.invites.pending.single.completeError(
      const ApiError(status: 410, message: 'This invite has expired.'),
    );
    await tester.pumpAndSettle();
    expect(find.text('This invite has expired.'), findsOneWidget);
    expect(find.text('Join group'), findsOneWidget);
  });

  testWidgets('another server offers to open the app instead', (tester) async {
    final other = await _open(
      tester,
      session: const Session(
        mode: AppMode.connected,
        serverUrl: 'https://other.example.com',
        token: 'tok',
      ),
    );
    expect(find.byKey(const Key('join-accept')), findsNothing);
    expect(find.textContaining('signed in to other.example.com'), findsOne);
    expect(find.text('Open Split Spends'), findsOneWidget);
    expect(other.invites.accepts, isEmpty);
  });

  testWidgets('on this device only, it offers to connect', (tester) async {
    await _open(tester, session: const Session(mode: AppMode.local));
    expect(find.byKey(const Key('join-accept')), findsNothing);
    expect(find.byKey(const Key('join-connect')), findsOneWidget);
  });
}
