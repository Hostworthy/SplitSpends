import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/account/presentation/account_screen.dart';

import '../../helpers.dart';

void main() {
  testWidgets('profile changes in on-device mode are saved on this device', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 2400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    disableSembastCooperator();
    final db = await pumpFuture(
      tester,
      databaseFactoryMemory.openDatabase(
        'test-${DateTime.now().microsecondsSinceEpoch}',
      ),
    );
    const prefs = NotificationPrefs();
    await pumpFuture(
      tester,
      Stores.meta
          .record('profile')
          .put(
            db,
            profileRecord(
              Me(
                id: 'local',
                name: 'Ada',
                defaultCurrency: 'EUR',
                timezone: 'Europe/Lisbon',
                notifications: prefs.copyWith(
                  email: prefs.email.set(NotificationKind.expenseAdded, false),
                ),
              ),
            ),
          ),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          localStoreProvider.overrideWithValue(
            MemoryLocalStore(appMode: AppMode.local.name),
          ),
          databaseProvider.overrideWith((ref) async => db),
        ],
        child: const MaterialApp(home: AccountScreen()),
      ),
    );
    await settle(tester);

    await tester.tap(find.byTooltip('Edit name'));
    await settle(tester);
    await tester.enterText(find.byType(TextField), 'Grace');
    await tester.tap(find.text('Save'));
    await settle(tester);
    await tester.tap(find.byType(CircleAvatar).at(1));
    await settle(tester);

    expect(find.text('Grace'), findsOneWidget);
    final stored = Me.fromJson(
      (await pumpFuture(tester, Stores.meta.record('profile').get(db)) as Map)
          .cast<String, Object?>(),
    );
    expect(stored.name, 'Grace');
    expect(stored.avatarColor, isNotNull);
    expect(stored.defaultCurrency, 'EUR');
    expect(stored.timezone, 'Europe/Lisbon');
    expect(
      stored.notifications.email.on(NotificationKind.expenseAdded),
      isFalse,
    );
  });

  test('a stored profile reads back as it was saved', () {
    const me = Me(
      id: 'u1',
      name: 'Ada',
      timezone: 'Europe/Lisbon',
      gravatar: true,
      role: 'admin',
    );
    final back = Me.fromJson(profileRecord(me));
    expect(back.timezone, 'Europe/Lisbon');
    expect(back.gravatar, isTrue);
    expect(back.role, 'admin');
  });
}
