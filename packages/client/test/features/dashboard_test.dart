import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/theme/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/dashboard/presentation/dashboard_screen.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';

import '../helpers.dart';

class _Groups extends Fake implements GroupsRepository {
  @override
  Future<List<Group>> list({bool archived = false}) async => const [];
}

class _Local extends SessionController {
  @override
  Future<Session> build() async => const Session(
    mode: AppMode.local,
    me: Me(id: 'ada', name: 'Ada'),
  );
}

void main() {
  testWidgets('the dashboard avatar is an Account button', (tester) async {
    final handle = tester.ensureSemantics();
    tester.view.physicalSize = const Size(420, 1600);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          memoryDatabase(),
          sessionProvider.overrideWith(_Local.new),
          groupsRepositoryProvider.overrideWithValue(_Groups()),
        ],
        child: MaterialApp(
          theme: AppTheme.light(),
          home: const DashboardScreen(),
        ),
      ),
    );
    await settle(tester);
    expect(
      tester.getSemantics(find.byTooltip('Account')),
      isSemantics(
        label: '',
        tooltip: 'Account',
        isButton: true,
        isFocusable: true,
        hasTapAction: true,
      ),
    );
    handle.dispose();
  });
}
