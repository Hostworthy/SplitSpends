import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/account/presentation/local_mode_card.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/sync/data/backup_service.dart';

import '../../helpers.dart';

const _profile = {'id': 'local', 'name': 'Ada', 'defaultCurrency': 'EUR'};

Future<Database> _open() async {
  final db = await databaseFactoryMemory.openDatabase(
    'test-${DateTime.now().microsecondsSinceEpoch}',
  );
  await Stores.meta.record('profile').put(db, _profile);
  return db;
}

Future<void> _camping(LocalLedger ledger) => LocalGroupsRepository(ledger)
    .create(
      const GroupInput(
        name: 'Camping',
        members: [MemberInput(name: 'Sam')],
      ),
    );

void main() {
  group('backup', () {
    test('restores what it exported and replaces the rest', () async {
      final db = await _open();
      final ledger = LocalLedger(() async => db);
      await _camping(ledger);
      final backup = BackupService(ledger);
      final bytes = await backup.export();

      await LocalGroupsRepository(ledger)
          .create(const GroupInput(name: 'Later'));
      await backup.restore(bytes);

      final groups = await ledger.groups(db);
      expect(groups.map((g) => g['name']), ['Camping']);
      expect((await ledger.people(db)).values.map((p) => p['name']), ['Sam']);
      expect((await ledger.me(db)).name, 'Ada');
    });

    test('a file that is not a backup changes nothing', () async {
      final db = await _open();
      final ledger = LocalLedger(() async => db);
      await _camping(ledger);
      final backup = BackupService(ledger);

      for (final bytes in [
        utf8.encode('not json'),
        utf8.encode(jsonEncode({'format': 'other', 'profile': _profile})),
        utf8.encode(
          jsonEncode({
            'format': backupFormat,
            'version': backupVersion + 1,
            'profile': _profile,
          }),
        ),
      ]) {
        await expectLater(
          backup.restore(Uint8List.fromList(bytes)),
          throwsA(isA<ApiError>()),
        );
      }
      expect(await ledger.groups(), hasLength(1));
    });
  });

  testWidgets('removing the on-device copy asks first', (tester) async {
    final db = await pumpFuture(tester, _open());
    final ledger = LocalLedger(() async => db);
    await pumpFuture(tester, _camping(ledger));
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          memoryDatabase(),
          localLedgerProvider.overrideWithValue(ledger),
        ],
        child: const MaterialApp(home: Scaffold(body: LocalCopyCard())),
      ),
    );
    await settle(tester);
    expect(find.text('On-device copy'), findsOneWidget);

    await tester.tap(find.text('Remove'));
    await settle(tester);
    await tester.tap(find.text('Keep'));
    await settle(tester);
    expect(await pumpFuture(tester, ledger.groups()), hasLength(1));

    await tester.tap(find.text('Remove'));
    await settle(tester);
    await tester.tap(find.widgetWithText(FilledButton, 'Remove'));
    await settle(tester);
    expect(await pumpFuture(tester, ledger.groups()), isEmpty);
    expect(
      await pumpFuture(tester, Stores.meta.record('profile').get(db)),
      isNull,
    );
    expect(find.text('On-device copy'), findsNothing);
    expect(find.text('On-device copy removed'), findsOneWidget);
  });
}
