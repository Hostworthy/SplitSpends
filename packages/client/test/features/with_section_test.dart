import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/expenses/presentation/editor/with_section.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

final _trip = Group.fromJson({
  'id': 'g1',
  'name': 'Trip',
  'kind': 'trip',
  'defaultCurrency': 'EUR',
  'simplifyDebts': true,
  'archived': false,
  'members': [
    {'id': 'ada', 'name': 'Ada'},
    {'id': 'sam', 'name': 'Sam'},
  ],
  'balances': <Object>[],
});

void main() {
  testWidgets('the group chip names the group before the list loads', (
    tester,
  ) async {
    final form = ExpenseForm(
      meId: 'ada',
      currency: 'EUR',
      groupId: 'g1',
      people: ['ada', 'sam'],
    );
    addTearDown(form.dispose);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          meProvider.overrideWithValue(const Me(id: 'ada', name: 'Ada')),
          groupsProvider.overrideWith((ref) => Completer<List<Group>>().future),
          groupProvider('g1').overrideWith((ref) async => _trip),
        ],
        child: MaterialApp(
          home: Scaffold(
            body: WithSection(
              form: form,
              people: {for (final m in _trip.members) m.id: m},
              onGroupMembers: (_) {},
            ),
          ),
        ),
      ),
    );
    await tester.pump();

    expect(find.widgetWithText(ActionChip, 'Trip'), findsOneWidget);
  });
}
