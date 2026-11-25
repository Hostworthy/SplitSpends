import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';

import '../helpers.dart';

void main() {
  test('opening an unversioned database keeps its records', () async {
    final name = 'versioned-${DateTime.now().microsecondsSinceEpoch}';
    final old = await databaseFactoryMemory.openDatabase(name);
    await Stores.people.record('p1').put(old, {'id': 'p1', 'name': 'Ada'});
    await old.close();

    final db = await openVersioned(databaseFactoryMemory, name);
    expect(db.version, schemaVersion);
    expect(await Stores.people.record('p1').get(db), {
      'id': 'p1',
      'name': 'Ada',
    });
    await db.close();
  });

  test('a fresh database starts at the current version', () async {
    final versions = <(int, int)>[];
    final db = await databaseFactoryMemory.openDatabase(
      'fresh-${DateTime.now().microsecondsSinceEpoch}',
      version: schemaVersion,
      onVersionChanged: (db, from, to) async {
        versions.add((from, to));
        await migrate(db, from, to);
      },
    );
    expect(versions, [(0, schemaVersion)]);
    await db.close();
  });

  test('a database from a newer app is refused with a message', () async {
    final name = 'newer-${DateTime.now().microsecondsSinceEpoch}';
    final newer = await databaseFactoryMemory.openDatabase(
      name,
      version: schemaVersion + 1,
    );
    await newer.close();
    await expectLater(
      openVersioned(databaseFactoryMemory, name),
      throwsA(
        isA<Object>().having(
          (e) => e.toString(),
          'message',
          contains('newer version'),
        ),
      ),
    );
  });

  testWidgets('an unreadable profile shows an error with a retry', (
    tester,
  ) async {
    disableSembastCooperator();
    final db = await tester.runAsync(
      () => databaseFactoryMemory.openDatabase(
        'broken-${DateTime.now().microsecondsSinceEpoch}',
      ),
    );
    await tester.runAsync(
      () => Stores.meta.record('profile').put(db!, {'id': 1}),
    );
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(
            MemoryLocalStore(appMode: AppMode.local.name),
          ),
          databaseProvider.overrideWith((ref) async => db!),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await settle(tester);
    expect(find.text('Could not open your data'), findsOneWidget);
    expect(find.text('Try again'), findsOneWidget);
    expect(find.text('Save backup'), findsOneWidget);

    await tester.runAsync(
      () => Stores.meta.record('profile').put(db!, {
        'id': 'ada',
        'name': 'Ada',
        'defaultCurrency': 'USD',
      }),
    );
    await tester.tap(find.text('Try again'));
    await settle(tester);
    expect(find.text('Could not open your data'), findsNothing);
  });
}
