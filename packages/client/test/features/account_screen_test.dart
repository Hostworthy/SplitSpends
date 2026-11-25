import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/account/presentation/account_screen.dart';

import '../helpers.dart';

class _ConnectedSession extends SessionController {
  @override
  Future<Session> build() async => const Session(
    mode: AppMode.connected,
    serverUrl: 'http://splitspends.test',
    token: 'tok',
    me: Me(id: 'u1', email: 'ada@example.com', name: 'Ada'),
  );
}

/// A server that takes [delay] to answer PATCH /me, keeps what it was sent
/// and records every PATCH body.
class _Server {
  _Server(this.delay, {this.fail = false});

  final Duration delay;

  /// Answers every PATCH with a server error.
  final bool fail;
  final patches = <Map<String, dynamic>>[];
  Map<String, dynamic> me = {
    'id': 'u1',
    'email': 'ada@example.com',
    'name': 'Ada',
    'defaultCurrency': 'USD',
  };

  http.Client get client => MockClient((request) async {
    if (request.method == 'PATCH' && request.url.path.endsWith('/me')) {
      final body = jsonDecode(request.body) as Map<String, dynamic>;
      patches.add(body);
      await Future<void>.delayed(delay);
      if (fail) {
        return http.Response(
          jsonEncode({'error': 'Something broke.', 'code': 'INTERNAL'}),
          500,
        );
      }
      me = {...me, ...body};
      return http.Response(jsonEncode({'data': me}), 200);
    }
    final Object body = request.url.path.endsWith('/config')
        ? {
            'data': {
              'appName': 'SplitSpends',
              'currencies': <Object>[],
              'categories': <Object>[],
            },
          }
        : {'items': <Object>[], 'total': 0};
    return http.Response(jsonEncode(body), 200);
  });
}

bool _selected(WidgetTester tester, NotificationKind kind, String channel) =>
    tester
        .widget<FilterChip>(
          find.descendant(
            of: find.widgetWithText(ListTile, kind.label),
            matching: find.widgetWithText(FilterChip, channel),
          ),
        )
        .selected;

Future<void> _tapChip(
  WidgetTester tester,
  NotificationKind kind,
  String channel,
) async {
  final chip = find.descendant(
    of: find.widgetWithText(ListTile, kind.label),
    matching: find.widgetWithText(FilterChip, channel),
  );
  await tester.ensureVisible(chip);
  await tester.tap(chip);
}

Future<void> _pump(WidgetTester tester, _Server server) async {
  tester.view.physicalSize = const Size(420, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        memoryDatabase(),
        sessionProvider.overrideWith(_ConnectedSession.new),
        httpClientProvider.overrideWithValue(server.client),
      ],
      child: const MaterialApp(home: AccountScreen()),
    ),
  );
  await settle(tester);
}

void main() {
  testWidgets('quick notification changes build on each other', (tester) async {
    final server = _Server(const Duration(milliseconds: 300));
    await _pump(tester, server);
    const a = NotificationKind.expenseAdded;
    const b = NotificationKind.paymentReceived;
    expect(_selected(tester, a, 'Email'), isTrue);
    expect(_selected(tester, b, 'Email'), isTrue);

    await _tapChip(tester, a, 'Email');
    await tester.pump(const Duration(milliseconds: 50));
    await _tapChip(tester, b, 'Email');
    await tester.pump(const Duration(milliseconds: 50));
    expect(_selected(tester, a, 'Email'), isFalse);
    expect(_selected(tester, b, 'Email'), isFalse);
    await settle(tester);

    expect(server.patches, hasLength(2));
    final last = server.patches.last['notifications'] as Map;
    expect(last[a.key], isFalse);
    expect(last[b.key], isFalse);
    expect(_selected(tester, a, 'Email'), isFalse);
    expect(_selected(tester, b, 'Email'), isFalse);
  });

  testWidgets('a failed change is undone and explained', (tester) async {
    final server = _Server(const Duration(milliseconds: 300), fail: true);
    await _pump(tester, server);
    const kind = NotificationKind.expenseAdded;

    await _tapChip(tester, kind, 'Email');
    await tester.pump(const Duration(milliseconds: 50));
    expect(_selected(tester, kind, 'Email'), isFalse);
    await settle(tester);

    expect(server.patches, hasLength(1));
    expect(_selected(tester, kind, 'Email'), isTrue);
    expect(find.text('Something broke.'), findsOneWidget);
  });
}
