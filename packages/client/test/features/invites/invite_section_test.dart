import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/invites/data/providers.dart';
import 'package:splitspends/features/invites/domain/invite.dart';
import 'package:splitspends/features/invites/presentation/invite_section.dart';

/// Fails the first list, then has no links.
class _Flaky extends Fake implements InvitesRepository {
  var lists = 0;

  @override
  Future<List<Invite>> list(String groupId) async {
    lists++;
    if (lists == 1) throw const ApiError.network('Cannot reach the server.');
    return const [];
  }
}

final _group = Group.fromJson({
  'id': 'g1',
  'name': 'Trip',
  'kind': 'trip',
  'defaultCurrency': 'EUR',
  'simplifyDebts': true,
  'archived': false,
  'members': [
    {'id': 'ada', 'name': 'Ada'},
  ],
  'balances': <Object>[],
});

void main() {
  testWidgets('links that failed to load can be loaded again', (tester) async {
    final flaky = _Flaky();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          appModeProvider.overrideWithValue(AppMode.connected),
          invitesRepositoryProvider.overrideWithValue(flaky),
        ],
        child: MaterialApp(
          home: Scaffold(
            body: ListView(children: [InviteSection(group: _group)]),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(
      find.text('Cannot reach the server. Check your connection.'),
      findsOneWidget,
    );

    await tester.tap(find.text('Try again'));
    await tester.pumpAndSettle();
    expect(flaky.lists, 2);
    expect(
      find.text('Cannot reach the server. Check your connection.'),
      findsNothing,
    );
  });
}
