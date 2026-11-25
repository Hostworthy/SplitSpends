import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/core/widgets/sync_bar.dart';

import '../helpers.dart';

const _server = 'http://splitspends.test';

/// Starts the signed-in app with one stuck write and opens its dialog;
/// returns the writes the server receives.
Future<List<String>> _openStuck(WidgetTester tester) async {
  final writes = <String>[];
  final api = MockClient((request) async {
    final path = request.url.path;
    if (path.endsWith('/me')) {
      return http.Response(
        jsonEncode({
          'data': {'id': 'u1', 'name': 'Ada', 'defaultCurrency': 'EUR'},
        }),
        200,
      );
    }
    if (request.method != 'GET') {
      writes.add('${request.method} $path');
      return http.Response(jsonEncode({'data': null}), 201);
    }
    final Object body = path.endsWith('/config')
        ? {
            'data': {
              'appName': 'SplitSpends',
              'currencies': <Object>[],
              'categories': <Object>[],
            },
          }
        : path.endsWith('/me/balances')
        ? {
            'data': {
              'balances': <Object>[],
              'owed': <Object>[],
              'owing': <Object>[],
            },
          }
        : {'items': <Object>[], 'total': 0};
    return http.Response(jsonEncode(body), 200);
  });
  tester.view.physicalSize = const Size(420, 2000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
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
        memoryDatabase(),
        httpClientProvider.overrideWithValue(api),
      ],
      child: const SplitSpendsApp(),
    ),
  );
  final container = ProviderScope.containerOf(
    tester.element(find.byType(SplitSpendsApp)),
  );
  final db = await pumpFuture(tester, container.read(databaseProvider.future));
  await pumpFuture(
    tester,
    Stores.outbox.add(db, {
      'method': 'POST',
      'path': '/expenses',
      'body': {'id': 'e1'},
      'label': 'Taxi',
      'server': _server,
      'user': 'u1',
      'attempts': Outbox.stuckAfter,
      'stuck': true,
      'error': 'Something went wrong. Please try again.',
    }),
  );
  await settle(tester);

  await tester.tap(find.textContaining('could not be sent'));
  await settle(tester);
  expect(find.text('Not sent yet'), findsOneWidget);
  return writes;
}

void main() {
  testWidgets('Retry on stuck changes closes only its own dialog', (
    tester,
  ) async {
    final writes = await _openStuck(tester);

    await tester.tap(find.widgetWithText(FilledButton, 'Retry'));
    for (var i = 0; i < 20; i++) {
      await tester.pump(const Duration(milliseconds: 10));
    }
    await settle(tester);

    expect(writes, ['POST /api/v1/expenses']);
    expect(find.text('Not sent yet'), findsNothing);
    expect(find.byType(SyncBar), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('discarding the last stuck change closes the dialog', (
    tester,
  ) async {
    final writes = await _openStuck(tester);

    await tester.tap(find.widgetWithText(TextButton, 'Discard'));
    for (var i = 0; i < 20; i++) {
      await tester.pump(const Duration(milliseconds: 10));
    }
    await settle(tester);

    expect(writes, isEmpty);
    expect(find.text('Not sent yet'), findsNothing);
    expect(find.textContaining('could not be sent'), findsNothing);
    expect(find.byType(SyncBar), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
