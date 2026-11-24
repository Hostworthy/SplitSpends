import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
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
    calls.add('save ${s.mode.name} ${s.allowed.join(',')}');
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

  @override
  Future<List<Account>> accounts({
    String status = '',
    String query = '',
  }) async => const [
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

Future<FakeAdmin> pump(WidgetTester tester, String role) async {
  tester.view.physicalSize = const Size(420, 1000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  final fake = FakeAdmin();
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

  testWidgets('everyone else sees nothing', (tester) async {
    await pump(tester, 'user');
    expect(find.text('Not available'), findsOneWidget);
  });
}
