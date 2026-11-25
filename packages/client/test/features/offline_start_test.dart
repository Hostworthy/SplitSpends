import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';

import '../helpers.dart';

const _server = 'http://splitspends.test';

void main() {
  testWidgets('an offline start can add an expense', (tester) async {
    tester.view.physicalSize = const Size(420, 2000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    // Signed in on an earlier run; this run starts without a connection.
    disableSembastCooperator();
    final db = (await tester.runAsync(() async {
      final db = await databaseFactoryMemory.openDatabase(
        'offline-${DateTime.now().microsecondsSinceEpoch}',
      );
      await Stores.meta.record('account').put(db, 'u1');
      await Stores.meta.record('accountProfile').put(db, {
        'id': 'u1',
        'name': 'Ada',
        'defaultCurrency': 'EUR',
      });
      return db;
    }))!;

    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(
            MemoryLocalStore(
              serverUrl: _server,
              token: 'tok',
              appMode: AppMode.connected.name,
            ),
          ),
          databaseProvider.overrideWith((ref) async => db),
          httpClientProvider.overrideWithValue(
            MockClient((_) => throw http.ClientException('offline')),
          ),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await settle(tester);

    await tester.tap(find.text('Add expense').first);
    await settle(tester);
    expect(find.byKey(const Key('expense-description')), findsOneWidget);
  });
}
