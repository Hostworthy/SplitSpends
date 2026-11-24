import 'dart:async';

// Runs the real app on an Android device or emulator: on-device mode first,
// then connecting to a server and uploading what was created offline.
//
//   flutter test integration_test -d <device> \
//     --dart-define=SPLITSPENDS_TEST_SERVER=http://10.0.2.2:8080 \
//     --dart-define=SPLITSPENDS_TEST_CODES=http://10.0.2.2:8099
//
// Without the defines only the on-device part runs. The codes URL is a small
// helper next to a development server (scripts/ci/codes.py) that returns the
// latest sign-in code it logged, so the test can sign in like a person.
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:integration_test/integration_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/main.dart' as client;

const server = String.fromEnvironment('SPLITSPENDS_TEST_SERVER');
const codes = String.fromEnvironment('SPLITSPENDS_TEST_CODES');
const email = String.fromEnvironment(
  'SPLITSPENDS_TEST_EMAIL',
  defaultValue: 'device-test@example.com',
);

/// Pumps real frames until [finder] matches, instead of pumpAndSettle,
/// which never settles while a spinner or network call is running.
Future<void> waitFor(
  WidgetTester tester,
  Finder finder, {
  Duration timeout = const Duration(seconds: 30),
}) async {
  step('waiting for $finder');
  final end = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(end)) {
    // A frame that never comes (screen off, app in the background) must
    // fail the test with a message instead of hanging it.
    await tester
        .pump(const Duration(milliseconds: 200))
        .timeout(const Duration(seconds: 10), onTimeout: () {});
    if (finder.evaluate().isNotEmpty) return;
  }
  throw TestFailure('Timed out waiting for $finder');
}

/// Progress lines, visible in the CI log and in logcat.
void step(String message) => debugPrint('device-test: $message');

Future<void> tapWhenReady(WidgetTester tester, Finder finder) async {
  await waitFor(tester, finder);
  await tester.ensureVisible(finder.first);
  await tester.pump(const Duration(milliseconds: 300));
  await tester.tap(finder.first);
  await tester.pump(const Duration(milliseconds: 300));
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
    'on this device, then connect and upload',
    timeout: const Timeout(Duration(minutes: 8)),
    (tester) async {
      step('starting the app');
      unawaited(client.main());
      await waitFor(tester, find.text('Use on this device'));

      // On-device mode: no account, no server.
      await tapWhenReady(tester, find.text('Start'));
      await waitFor(tester, find.byKey(const Key('local-name')));
      await tester.enterText(find.byKey(const Key('local-name')), 'Ada');
      await tapWhenReady(tester, find.byKey(const Key('local-start')));
      await waitFor(tester, find.text('Total balance'));

      final container = ProviderScope.containerOf(
        tester.element(find.byType(SplitSpendsApp)),
      );
      final group = await container
          .read(groupsRepositoryProvider)
          .create(
            const GroupInput(
              name: 'Cabin',
              members: [
                MemberInput(name: 'Sam'),
                MemberInput(name: 'Kim'),
              ],
            ),
          );

      // Add \$90 groceries through the editor, split equally three ways.
      final router = container.read(routerProvider);
      unawaited(router.push('/expenses/new?groupId=${group.id}'));
      await waitFor(tester, find.byKey(const Key('expense-description')));
      await tester.enterText(
        find.byKey(const Key('expense-description')),
        'Groceries',
      );
      await tester.enterText(
        find.descendant(
          of: find.byType(AmountField).first,
          matching: find.byType(TextField),
        ),
        '90',
      );
      await waitFor(tester, find.text('Ready to save'));
      await tapWhenReady(tester, find.text('Save'));

      router.go('/');
      await waitFor(tester, find.text('You are owed'));
      expect(find.text(r'$60.00'), findsWidgets);

      if (server.isEmpty || codes.isEmpty) {
        markTestSkipped('No test server given; on-device part only.');
        return;
      }

      // Connect to the server from Account, signing in with an emailed code.
      router.go('/account');
      await tapWhenReady(tester, find.byKey(const Key('connect-from-local')));
      await waitFor(tester, find.byKey(const Key('login-email')));
      final serverField = find.byKey(const Key('login-server'));
      if (serverField.evaluate().isNotEmpty) {
        await tester.enterText(serverField, server);
      }
      await tester.enterText(find.byKey(const Key('login-email')), email);
      await tapWhenReady(tester, find.byKey(const Key('login-continue')));
      await waitFor(tester, find.byKey(const Key('otp-field')));

      step('fetching the sign-in code');
      final res = await http
          .get(Uri.parse('$codes/code?email=$email'))
          .timeout(const Duration(seconds: 20));
      expect(res.statusCode, 200, reason: res.body);
      final code = (jsonDecode(res.body) as Map)['code'] as String;
      await tester.enterText(find.byKey(const Key('otp-field')), code);

      // New accounts are asked for a name; then the offline data is uploaded.
      await waitFor(
        tester,
        find.byWidgetPredicate(
          (w) =>
              w.key == const Key('name-field') ||
              w.key == const Key('import-upload'),
        ),
      );
      if (find.byKey(const Key('name-field')).evaluate().isNotEmpty) {
        await tester.enterText(find.byKey(const Key('name-field')), 'Ada');
        await tapWhenReady(tester, find.text('Get started'));
      }
      await tapWhenReady(tester, find.byKey(const Key('import-upload')));
      step('uploading');

      // The same balance, now served by the server.
      await waitFor(
        tester,
        find.text('Cabin'),
        timeout: const Duration(seconds: 60),
      );
      router.go('/');
      await waitFor(tester, find.text('You are owed'));
      expect(find.text(r'$60.00'), findsWidgets);
    },
  );
}
