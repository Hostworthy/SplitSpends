import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/presentation/group_form_sheet.dart';

/// Fails every create, as if the answer was lost after it reached the server.
class _Flaky extends Fake implements GroupsRepository {
  final ids = <String?>[];

  @override
  Future<Group> create(GroupInput input) async {
    ids.add(input.id);
    throw const ApiError.network('Cannot reach the server.');
  }
}

void main() {
  testWidgets('creating again after a failure keeps the same group id', (
    tester,
  ) async {
    final flaky = _Flaky();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          appModeProvider.overrideWithValue(AppMode.connected),
          meProvider.overrideWithValue(const Me(id: 'ada', name: 'Ada')),
          groupsRepositoryProvider.overrideWithValue(flaky),
        ],
        child: MaterialApp(
          home: Builder(
            builder: (context) => Scaffold(
              body: TextButton(
                onPressed: () => showNewGroupSheet(context),
                child: const Text('Open'),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.widgetWithText(TextField, 'Group name'),
      'Flat',
    );

    await tester.tap(find.text('Create group'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Create group'));
    await tester.pumpAndSettle();

    expect(flaky.ids, hasLength(2));
    expect(flaky.ids.first, isNotNull);
    expect(flaky.ids.last, flaky.ids.first);
  });
}
