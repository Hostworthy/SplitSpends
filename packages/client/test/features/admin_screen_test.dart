import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/features/admin/domain/admin.dart';
import 'package:splitspends/features/admin/presentation/admin_screen.dart';

class FakeAdmin implements AdminRepository {
  final calls = <String>[];
  var settings = const SignupSettings(mode: SignupMode.invite);
  final pending = [
    SignupRequest(
      id: 'r1',
      email: 'dan@else.org',
      name: 'Dan',
      status: 'pending',
      requestedAt: DateTime.utc(2026, 11, 24),
    ),
  ];

  @override
  Future<SignupSettings> signup() async => settings;

  @override
  Future<SignupSettings> saveSignup(SignupSettings s) async {
    calls.add('save ${s.toJson()['mode']} ${s.allowed.join(',')}');
    return settings = s;
  }

  @override
  Future<List<SignupRequest>> requests({String status = 'pending'}) async =>
      status == 'pending' ? pending : const [];

  @override
  Future<SignupRequest> approve(String id) async {
    calls.add('approve $id');
    pending.clear();
    return SignupRequest(
      id: id,
      email: 'dan@else.org',
      name: 'Dan',
      status: 'approved',
      requestedAt: DateTime.utc(2026, 11, 24),
    );
  }

  @override
  Future<SignupRequest> decline(String id) async => throw UnimplementedError();

  /// How many accounts the server has; the list holds the first ones.
  int total = 2;
  List<Account> people = const [
    Account(
      user: User(id: 'u1', email: 'ada@example.com', name: 'Ada'),
      role: 'admin',
    ),
    Account(
      user: User(id: 'u2', email: 'ben@example.com', name: 'Ben'),
      role: 'user',
    ),
  ];

  @override
  Future<PageResult<Account>> accounts({
    String status = '',
    String query = '',
  }) async => PageResult(items: people, total: total);

  @override
  Future<Account> setRole(String userId, String role) async {
    calls.add('role $userId $role');
    return Account(
      user: const User(id: 'u2', name: 'Ben'),
      role: role,
    );
  }

  @override
  Future<Account> disable(String userId) async => throw UnimplementedError();

  @override
  Future<Account> enable(String userId) async => throw UnimplementedError();
}

Future<FakeAdmin> pump(
  WidgetTester tester,
  String role, {
  FakeAdmin? admin,
}) async {
  tester.view.physicalSize = const Size(420, 1000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  final fake = admin ?? FakeAdmin();
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        meProvider.overrideWithValue(
          Me(id: 'u1', email: 'ada@example.com', name: 'Ada', role: role),
        ),
        adminRepositoryProvider.overrideWithValue(fake),
      ],
      child: MaterialApp(theme: AppTheme.light(), home: const AdminScreen()),
    ),
  );
  await tester.pumpAndSettle();
  return fake;
}

void main() {
  testWidgets('managers only see approvals and can approve', (tester) async {
    final fake = await pump(tester, 'manager');
    expect(find.text('Approvals'), findsOneWidget);
    expect(find.text('Sign-up'), findsNothing);
    expect(find.text('Dan'), findsOneWidget);
    await tester.tap(find.text('Approve'));
    await tester.pumpAndSettle();
    expect(fake.calls, ['approve r1']);
    expect(find.text('Nobody is waiting'), findsOneWidget);
  });

  testWidgets('administrators change the mode and the allowed list', (
    tester,
  ) async {
    final fake = await pump(tester, 'admin');
    expect(find.text('Administration'), findsOneWidget);
    await tester.tap(find.text('Sign-up'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('mode-approval')));
    await tester.enterText(
      find.byKey(const Key('allowed-entry')),
      '@Family.com',
    );
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();
    expect(find.text('@family.com'), findsOneWidget);
    await tester.ensureVisible(find.byKey(const Key('save-signup')));
    await tester.tap(find.byKey(const Key('save-signup')));
    await tester.pumpAndSettle();
    expect(fake.calls, ['save approval @family.com']);
  });

  testWidgets('a sign-up mode this app does not know is kept', (tester) async {
    final admin = FakeAdmin()
      ..settings = SignupSettings.fromJson({
        'mode': 'future',
        'allowed': <String>[],
      });
    final fake = await pump(tester, 'admin', admin: admin);
    // Room for the notice above the modes and the Save button below.
    tester.view.physicalSize = const Size(420, 1400);
    await tester.tap(find.text('Sign-up'));
    await tester.pumpAndSettle();

    expect(find.textContaining('does not know'), findsOneWidget);
    for (final m in SignupMode.values) {
      final tile = tester.widget<RadioListTile<SignupMode>>(
        find.byKey(Key('mode-${m.name}')),
      );
      // ignore: deprecated_member_use
      expect(tile.groupValue, isNull, reason: m.name);
    }

    await tester.enterText(
      find.byKey(const Key('allowed-entry')),
      'ben@example.com',
    );
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();
    expect(find.text('ben@example.com'), findsOneWidget);
    await tester.ensureVisible(find.byKey(const Key('save-signup')));
    await tester.tap(find.byKey(const Key('save-signup')));
    await tester.pumpAndSettle();
    expect(fake.calls, ['save future ben@example.com']);
  });

  testWidgets('administrators change roles', (tester) async {
    final fake = await pump(tester, 'admin');
    await tester.tap(find.text('People'));
    await tester.pumpAndSettle();
    expect(find.text('Admin'), findsOneWidget);
    await tester.tap(find.text('Ben'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Manager (approves people)'));
    await tester.pumpAndSettle();
    expect(fake.calls, ['role u2 manager']);
  });

  testWidgets('a long account list says it is not complete', (tester) async {
    final admin = FakeAdmin()
      ..total = 250
      ..people = [
        for (var i = 0; i < 200; i++)
          Account(
            user: User(id: 'u$i', name: 'Person $i'),
            role: 'user',
          ),
      ];
    await pump(tester, 'admin', admin: admin);
    await tester.tap(find.text('People'));
    await tester.pumpAndSettle();
    final hint = find.text('Showing 200 of 250. Search to narrow the list.');
    await tester.scrollUntilVisible(
      hint,
      500,
      scrollable: find
          .descendant(
            of: find.byType(ListView),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    expect(hint, findsOneWidget);
  });

  testWidgets('a complete account list has no hint', (tester) async {
    await pump(tester, 'admin');
    await tester.tap(find.text('People'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Search to narrow'), findsNothing);
  });

  testWidgets('everyone else sees nothing', (tester) async {
    await pump(tester, 'user');
    expect(find.text('Not available'), findsOneWidget);
  });
}
