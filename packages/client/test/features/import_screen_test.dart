import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/models/json.dart';
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

class _FailingImport implements ImportService {
  @override
  Future<ImportSummary> summary() async =>
      const ImportSummary(groups: 1, expenses: 2, people: 1);

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

void main() {
  testWidgets('a failed upload shows the error under the button', (
    tester,
  ) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          memoryDatabase(),
          sessionProvider.overrideWith(_LocalSession.new),
          importServiceProvider.overrideWithValue(_FailingImport()),
        ],
        child: const MaterialApp(home: ImportScreen(login: _login)),
      ),
    );
    await settle(tester);

    await tester.tap(find.byKey(const Key('import-upload')));
    await settle(tester);

    expect(find.text('Something went wrong.'), findsOneWidget);
    final upload = tester.widget<ButtonStyleButton>(
      find.byKey(const Key('import-upload')),
    );
    expect(upload.onPressed, isNotNull);
  });
}
