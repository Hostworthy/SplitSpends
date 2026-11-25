import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/features/sync/data/providers.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';
import 'package:splitspends/features/sync/data/import_service.dart';
import 'package:splitspends/features/sync/domain/import_payload.dart';
import 'package:splitspends/features/sync/presentation/import_screen.dart';

import '../helpers.dart';

const _login = LoginResult(
  token: 'tok',
  me: Me(id: 'u1', name: 'Ada'),
  isNew: false,
);

final _uploadedAt = DateTime(2026, 11, 20, 10);

class _FakeImport implements ImportService {
  _FakeImport({
    this.summaryValue = const ImportSummary(groups: 1, expenses: 2, people: 1),
  });

  final ImportSummary summaryValue;

  @override
  Future<ImportSummary> summary() async => summaryValue;

  @override
  Future<void> upload({required String serverUrl, required String token}) =>
      throw StateError('unreadable local expense');

  @override
  Future<bool> hasProfile() async => true;

  @override
  Future<JsonMap> payload() async => const {};

  @override
  Future<void> removeLocalCopy() async {}
}

class _LocalSession extends SessionController {
  @override
  Future<Session> build() async => const Session(
    mode: AppMode.local,
    serverUrl: 'http://splitspends.test',
    me: Me(id: 'local', name: 'Ada'),
  );
}

Future<void> _pump(
  WidgetTester tester,
  ImportService service, {
  _LocalSession Function()? session,
}) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        memoryDatabase(),
        sessionProvider.overrideWith(session ?? _LocalSession.new),
        importServiceProvider.overrideWithValue(service),
      ],
      child: const MaterialApp(home: ImportScreen(login: _login)),
    ),
  );
  await settle(tester);
}

/// A device that cannot store the session token.
class _BrokenSignIn extends _LocalSession {
  @override
  Future<void> signIn({required String token, required Me me}) async =>
      throw StateError('keystore unavailable');
}

void main() {
  test('the summary says where the data was uploaded', () async {
    final db = await databaseFactoryMemory.openDatabase(
      'test-${DateTime.now().microsecondsSinceEpoch}',
    );
    await Stores.meta.record('profile').put(db, {'id': 'local', 'name': 'Ada'});
    final service = ImportService(
      LocalLedger(() async => db),
      MockClient((_) async => http.Response('', 500)),
    );
    expect((await service.summary()).uploaded, isNull);

    await Stores.meta.record('importedTo').put(db, {
      'serverUrl': 'http://splitspends.test',
      'at': '2026-11-20T10:00:00.000Z',
    });
    final uploaded = (await service.summary()).uploaded;
    expect(uploaded?.server, 'http://splitspends.test');
    expect(uploaded?.at, DateTime.utc(2026, 11, 20, 10));
    expect(uploaded?.user, isNull, reason: 'kept before accounts were noted');
  });

  group('upload', () {
    Future<Database> seeded() async {
      final db = await databaseFactoryMemory.openDatabase(
        'test-${DateTime.now().microsecondsSinceEpoch}',
      );
      await Stores.meta.record('profile').put(db, {
        'id': 'local',
        'name': 'Ada',
      });
      await Stores.people.record('p1').put(db, {
        'id': 'p1',
        'name': 'Sam',
        'createdAt': '2026-11-20T10:00:00Z',
      });
      await Stores.groups.record('g1').put(db, {
        'id': 'g1',
        'name': 'Camping',
        'defaultCurrency': 'EUR',
        'memberIds': ['local', 'p1'],
      });
      return db;
    }

    test('rewrites people ids with the ones the server returns', () async {
      final db = await seeded();
      final sent = <http.Request>[];
      final service = ImportService(
        LocalLedger(() async => db),
        MockClient((request) async {
          sent.add(request);
          return http.Response(
            jsonEncode({
              'data': {
                'people': {'local': 'server-ada', 'p1': 'server-sam'},
              },
            }),
            200,
          );
        }),
      );

      await service.upload(serverUrl: 'http://splitspends.test', token: 'tok');

      expect(sent.single.url.path, '/api/v1/import');
      expect(sent.single.headers['Authorization'], 'Bearer tok');
      expect(await Stores.people.findKeys(db), ['server-sam']);
      expect((await Stores.groups.record('g1').get(db))?['memberIds'], [
        'server-ada',
        'server-sam',
      ]);
      expect(
        (await Stores.meta.record('profile').get(db) as Map?)?['id'],
        'server-ada',
      );
      final uploaded = (await service.summary()).uploaded;
      expect(uploaded?.server, 'http://splitspends.test');
      expect(uploaded?.user, 'server-ada', reason: 'the account it went to');
    });

    test('a failed upload keeps the local copy as it was', () async {
      final db = await seeded();
      final service = ImportService(
        LocalLedger(() async => db),
        MockClient(
          (_) async => http.Response(
            jsonEncode({'error': 'Something broke.', 'code': 'INTERNAL'}),
            500,
          ),
        ),
      );

      await expectLater(
        service.upload(serverUrl: 'http://splitspends.test', token: 'tok'),
        throwsA(isA<ApiError>()),
      );

      expect(await Stores.people.findKeys(db), ['p1']);
      expect((await Stores.groups.record('g1').get(db))?['memberIds'], [
        'local',
        'p1',
      ]);
      expect(
        (await Stores.meta.record('profile').get(db) as Map?)?['id'],
        'local',
      );
      expect((await service.summary()).uploaded, isNull);
    });
  });

  testWidgets('a second sign-in after an upload says the data was uploaded', (
    tester,
  ) async {
    await _pump(
      tester,
      _FakeImport(
        summaryValue: ImportSummary(
          groups: 1,
          uploaded: (
            server: 'http://splitspends.test/',
            at: _uploadedAt,
            user: 'u1',
          ),
        ),
      ),
    );

    expect(
      find.text(
        'You already uploaded this data to this server on '
        '${Dates.short(_uploadedAt)}.',
      ),
      findsOneWidget,
    );
    expect(
      find.widgetWithText(FilledButton, 'Start fresh (keep local copy)'),
      findsOneWidget,
    );
    expect(find.widgetWithText(OutlinedButton, 'Upload again'), findsOneWidget);
  });

  testWidgets('an upload to another account keeps Start fresh first', (
    tester,
  ) async {
    await _pump(
      tester,
      _FakeImport(
        summaryValue: ImportSummary(
          groups: 1,
          uploaded: (
            server: 'http://splitspends.test',
            at: _uploadedAt,
            user: 'someone-else',
          ),
        ),
      ),
    );

    expect(
      find.text(
        'You uploaded this data to another account on this server on '
        '${Dates.short(_uploadedAt)}. Its groups and expenses are already '
        'there, in that account.',
      ),
      findsOneWidget,
    );
    expect(
      find.widgetWithText(FilledButton, 'Start fresh (keep local copy)'),
      findsOneWidget,
    );
    expect(find.widgetWithText(OutlinedButton, 'Upload'), findsOneWidget);
  });

  testWidgets('an upload to another server is mentioned by name', (
    tester,
  ) async {
    await _pump(
      tester,
      _FakeImport(
        summaryValue: ImportSummary(
          groups: 1,
          uploaded: (
            server: 'https://other.example.com',
            at: _uploadedAt,
            user: 'u1',
          ),
        ),
      ),
    );

    expect(
      find.text(
        'You uploaded this data to other.example.com on '
        '${Dates.short(_uploadedAt)}.',
      ),
      findsOneWidget,
    );
    expect(find.widgetWithText(FilledButton, 'Upload'), findsOneWidget);
  });

  testWidgets('a failed upload shows the error under the button', (
    tester,
  ) async {
    await _pump(tester, _FakeImport());

    await tester.tap(find.byKey(const Key('import-upload')));
    await settle(tester);

    expect(find.text('Something went wrong.'), findsOneWidget);
    final upload = tester.widget<ButtonStyleButton>(
      find.byKey(const Key('import-upload')),
    );
    expect(upload.onPressed, isNotNull);
  });

  testWidgets('a failed start-fresh sign-in shows the error', (tester) async {
    await _pump(tester, _FakeImport(), session: _BrokenSignIn.new);

    await tester.tap(find.byKey(const Key('import-skip')));
    await settle(tester);

    expect(find.text('Something went wrong.'), findsOneWidget);
    final skip = tester.widget<ButtonStyleButton>(
      find.byKey(const Key('import-skip')),
    );
    expect(skip.onPressed, isNotNull);
  });
}
