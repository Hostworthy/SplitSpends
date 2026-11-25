import 'dart:async';
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';

const _server = 'http://server.test';
const _ada = Me(id: 'ada', name: 'Ada');
const _bob = Me(id: 'bob', name: 'Bob');

/// A server that knows which token belongs to whom and records every write.
class _Server {
  final tokens = <String, Me>{};
  final writes = <String>[];

  /// When set, holds every write until it completes.
  Completer<void>? gate;

  http.Client client() => MockClient((request) async {
    final token = request.headers['Authorization']?.substring('Bearer '.length);
    final me = tokens[token];
    if (me == null) {
      return http.Response(
        jsonEncode({'error': 'Please sign in again.', 'code': 'UNAUTHORIZED'}),
        401,
      );
    }
    if (request.method == 'GET' && request.url.path.endsWith('/me')) {
      return http.Response(jsonEncode({'data': me.toJson()}), 200);
    }
    await gate?.future;
    writes.add('${me.id} ${request.method} ${request.url.path}');
    return http.Response(jsonEncode({'data': null}), 201);
  });
}

void main() {
  late _Server server;
  late MemoryLocalStore store;
  late Database db;

  setUp(() async {
    server = _Server();
    store = MemoryLocalStore(serverUrl: _server);
    db = await databaseFactoryMemory.openDatabase(
      'session-${DateTime.now().microsecondsSinceEpoch}',
    );
  });

  ProviderContainer start() {
    final container = ProviderContainer(
      overrides: [
        localStoreProvider.overrideWithValue(store),
        httpClientProvider.overrideWithValue(server.client()),
        databaseProvider.overrideWith((ref) async => db),
      ],
    );
    addTearDown(container.dispose);
    // Keep the replay running, as the app shell does.
    container.listen(outboxSyncProvider, (_, _) {});
    return container;
  }

  Outbox outbox(Me me) =>
      Outbox(() async => db, owner: (server: _server, user: me.id));

  Future<void> signIn(ProviderContainer container, Me me, String token) async {
    server.tokens[token] = me;
    await container.read(sessionProvider.notifier).signIn(token: token, me: me);
    await pumpEventQueue();
  }

  test(
    'an expired session keeps unsynced writes for the same account',
    () async {
      final container = start();
      await container.read(sessionProvider.future);
      await signIn(container, _ada, 'first');
      // Written while offline.
      await container
          .read(outboxProvider)
          .add('POST', '/expenses', body: {'id': 'e1'}, label: 'Taxi');

      // The server ends the session.
      server.tokens.remove('first');
      await expectLater(
        container.read(apiClientProvider).getData('/me', (d) => d),
        throwsA(isA<ApiError>()),
      );
      await pumpEventQueue();

      final session = container.read(sessionProvider).requireValue;
      expect(session.token, isNull);
      expect(store.token, isNull);
      expect(server.writes, isEmpty);
      expect((await outbox(_ada).pending()).single.label, 'Taxi');

      await signIn(container, _ada, 'second');
      expect(server.writes, ['ada POST /api/v1/expenses']);
      expect(await outbox(_ada).pending(), isEmpty);
    },
  );

  test('a user sign-out discards only the account own writes', () async {
    await outbox(_bob).add('POST', '/expenses', body: {'id': 'b1'}, label: 'B');
    final container = start();
    await container.read(sessionProvider.future);
    await signIn(container, _ada, 'first');
    await container
        .read(outboxProvider)
        .add('POST', '/expenses', body: {'id': 'e1'}, label: 'A');

    await container.read(sessionProvider.notifier).signOut();
    expect(await outbox(_ada).pending(), isEmpty);
    expect((await outbox(_bob).pending()).single.label, 'B');
  });

  test('another account signing in never replays earlier writes', () async {
    // Ada's session ended at start-up with a change still queued.
    await outbox(_ada).add('POST', '/expenses', body: {'id': 'e1'}, label: 'A');
    await Stores.meta.record('account').put(db, _ada.id);
    store
      ..token = 'expired'
      ..appMode = AppMode.connected.name;
    final container = start();
    expect((await container.read(sessionProvider.future)).token, isNull);

    await signIn(container, _bob, 'bob');
    expect(server.writes, isEmpty);
    container
      ..listen(outboxCountProvider, (_, _) {})
      ..listen(outboxOthersProvider, (_, _) {});
    expect(await container.read(outboxCountProvider.future), 0);
    expect(await container.read(outboxOthersProvider.future), 1);
    expect((await outbox(_ada).pending()).single.label, 'A');

    await container.read(sessionProvider.notifier).signOut();
    await signIn(container, _ada, 'ada');
    expect(server.writes, [
      'bob POST /api/v1/auth/logout',
      'ada POST /api/v1/expenses',
    ]);
  });

  test('an offline start still knows whose writes it queues', () async {
    await Stores.meta.record('account').put(db, _ada.id);
    store
      ..token = 'offline'
      ..appMode = AppMode.connected.name;
    final container = ProviderContainer(
      overrides: [
        localStoreProvider.overrideWithValue(store),
        httpClientProvider.overrideWithValue(
          MockClient((_) => throw http.ClientException('offline')),
        ),
        databaseProvider.overrideWith((ref) async => db),
      ],
    );
    addTearDown(container.dispose);

    final session = await container.read(sessionProvider.future);
    expect(session.me, isNull);
    expect(session.owner, (server: _server, user: _ada.id));
  });

  group('OutboxSync', () {
    test('overlapping triggers send each write once', () async {
      final container = start();
      await container.read(sessionProvider.future);
      await signIn(container, _ada, 'first');
      server.gate = Completer();
      final queue = container.read(outboxProvider);
      await queue.add('POST', '/expenses', body: {'id': 'e1'}, label: '1');
      await queue.add('POST', '/expenses', body: {'id': 'e2'}, label: '2');

      final sync = container.read(outboxSyncProvider.notifier);
      final passes = [sync.flush(), sync.flush(), sync.retry()];
      await pumpEventQueue();
      server.gate!.complete();
      await Future.wait(passes);
      await pumpEventQueue();

      expect(server.writes, [
        'ada POST /api/v1/expenses',
        'ada POST /api/v1/expenses',
      ]);
      expect(await queue.pending(), isEmpty);
    });

    test('a pass that fails unexpectedly does not stop later passes', () async {
      final failing = _Failing(
        () async => db,
        owner: (server: _server, user: _ada.id),
      );
      final container = ProviderContainer(
        overrides: [
          localStoreProvider.overrideWithValue(store),
          httpClientProvider.overrideWithValue(server.client()),
          databaseProvider.overrideWith((ref) async => db),
          outboxProvider.overrideWithValue(failing),
        ],
      );
      addTearDown(container.dispose);
      container.listen(outboxSyncProvider, (_, _) {});
      await container.read(sessionProvider.future);
      await failing.add('POST', '/expenses', body: {'id': 'e1'}, label: '1');
      await pumpEventQueue();
      failing.passes = 0;

      final sync = container.read(outboxSyncProvider.notifier);
      await expectLater(sync.flush(), throwsStateError);
      expect(container.read(outboxSyncProvider).syncing, isFalse);
      await expectLater(sync.flush(), throwsStateError);
      expect(failing.passes, 2, reason: 'the second pass ran');
    });

    test('Retry-After pauses automatic replay; Retry overrides it', () async {
      var limited = true;
      final container = ProviderContainer(
        overrides: [
          localStoreProvider.overrideWithValue(store),
          databaseProvider.overrideWith((ref) async => db),
          httpClientProvider.overrideWithValue(
            MockClient((request) async {
              if (request.url.path.endsWith('/me')) {
                return http.Response(jsonEncode({'data': _ada.toJson()}), 200);
              }
              if (limited) {
                return http.Response(
                  'Slow down',
                  429,
                  headers: {'retry-after': '600'},
                );
              }
              server.writes.add('${request.method} ${request.url.path}');
              return http.Response(jsonEncode({'data': null}), 201);
            }),
          ),
        ],
      );
      addTearDown(container.dispose);
      container.listen(outboxSyncProvider, (_, _) {});
      await container.read(sessionProvider.future);
      await container
          .read(sessionProvider.notifier)
          .signIn(token: 't', me: _ada);
      await container
          .read(outboxProvider)
          .add('POST', '/expenses', body: {'id': 'e1'}, label: '1');

      final sync = container.read(outboxSyncProvider.notifier);
      await sync.flush();
      limited = false;
      await sync.flush();
      expect(server.writes, isEmpty, reason: 'the server asked for a pause');

      await sync.retry();
      expect(server.writes, ['POST /api/v1/expenses']);
    });
  });
}

/// An outbox whose replay fails with something other than an [ApiError].
class _Failing extends Outbox {
  _Failing(super.open, {required super.owner});

  var passes = 0;

  @override
  Future<FlushResult> flush(ApiClient api) async {
    passes++;
    throw StateError('The local database is closed.');
  }
}
