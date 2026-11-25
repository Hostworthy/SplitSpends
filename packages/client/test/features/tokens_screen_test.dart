import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/account/domain/account_repository.dart';
import 'package:splitspends/features/account/domain/api_token.dart';
import 'package:splitspends/features/account/presentation/tokens_screen.dart';

import '../helpers.dart';

class _FakeTokens implements TokensRepository {
  final tokens = <ApiToken>[
    const ApiToken(
      id: 't1',
      name: 'Claude Desktop',
      createdAt: '2026-11-20T10:00:00Z',
    ),
  ];
  final revoked = <String>[];

  @override
  Future<List<ApiToken>> list() async => List.of(tokens);

  @override
  Future<ApiToken> create(String name) async {
    final token = ApiToken(
      id: 't${tokens.length + 1}',
      name: name,
      createdAt: '2026-11-25T10:00:00Z',
      token: 'ss_secret_value',
    );
    tokens.add(token);
    return token;
  }

  @override
  Future<void> revoke(String id) async {
    revoked.add(id);
    tokens.removeWhere((t) => t.id == id);
  }
}

Future<void> _pump(WidgetTester tester, _FakeTokens tokens) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        memoryDatabase(),
        tokensRepositoryProvider.overrideWithValue(tokens),
      ],
      child: const MaterialApp(home: TokensScreen()),
    ),
  );
  await settle(tester);
}

void main() {
  testWidgets('token dialog stays until Done', (tester) async {
    await _pump(tester, _FakeTokens());

    await tester.tap(find.text('New token'));
    await settle(tester);
    await tester.tap(find.text('Create'));
    await settle(tester);
    expect(find.text('ss_secret_value'), findsOneWidget);

    await tester.tapAt(const Offset(5, 5));
    await settle(tester);
    expect(find.text('ss_secret_value'), findsOneWidget);

    await tester.binding.handlePopRoute();
    await settle(tester);
    expect(find.text('ss_secret_value'), findsOneWidget);

    await tester.tap(find.text('Done'));
    await settle(tester);
    expect(find.text('ss_secret_value'), findsNothing);
  });

  testWidgets('revoke asks first', (tester) async {
    final tokens = _FakeTokens();
    await _pump(tester, tokens);

    await tester.tap(find.text('Revoke'));
    await settle(tester);
    expect(find.text('Revoke "Claude Desktop"?'), findsOneWidget);
    expect(tokens.revoked, isEmpty);

    await tester.tap(find.text('Keep'));
    await settle(tester);
    expect(tokens.revoked, isEmpty);
    expect(find.text('Claude Desktop'), findsOneWidget);

    await tester.tap(find.text('Revoke'));
    await settle(tester);
    await tester.tap(find.widgetWithText(FilledButton, 'Revoke'));
    await settle(tester);
    expect(tokens.revoked, ['t1']);
    expect(find.text('Claude Desktop'), findsNothing);
  });
}
