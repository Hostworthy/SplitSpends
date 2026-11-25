import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/features/reminders/data/providers.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/reminders/domain/reminder.dart';
import 'package:splitspends/features/reminders/presentation/remind_button.dart';

class FakeReminders implements RemindersRepository {
  final sent = <String>[];
  bool emailed = true;
  bool limited = false;

  @override
  Future<Reminder> send(String userId, {String? groupId}) async {
    if (limited) {
      throw const ApiError(
        status: 429,
        code: 'RATE_LIMITED',
        message: 'You already reminded Ben in the last 24 hours. Try again tomorrow.',
      );
    }
    sent.add('$userId|${groupId ?? ''}');
    return Reminder(
      id: 'r1',
      userId: userId,
      groupId: groupId,
      emailed: emailed,
    );
  }
}

const _ben = User(id: 'ben', email: 'ben@example.com', name: 'Ben Lee');
const _sam = User(id: 'sam', name: 'Sam', status: 'placeholder');
const _owed = [Balance(currency: 'EUR', amount: 3000)];

Future<void> _pump(
  WidgetTester tester,
  User user,
  AppMode mode,
  FakeReminders reminders,
) => tester.pumpWidget(
  ProviderScope(
    overrides: [
      appModeProvider.overrideWithValue(mode),
      remindersRepositoryProvider.overrideWithValue(reminders),
    ],
    child: MaterialApp(
      home: Scaffold(
        body: Center(
          child: RemindButton(
            user: user,
            owed: _owed,
            groupId: 'g1',
            groupName: 'Flat',
            compact: false,
          ),
        ),
      ),
    ),
  ),
);

void main() {
  test('reminder text lists every amount and the group', () {
    expect(joinAmounts([]), '');
    expect(joinAmounts([r'$1.00']), r'$1.00');
    expect(joinAmounts(['€1.00', r'$2.00']), r'€1.00 and $2.00');
    expect(
      joinAmounts(['€1.00', '£2.00', r'$3.00']),
      r'€1.00, £2.00 and $3.00',
    );
    expect(
      reminderText(name: 'Ben', amounts: ['€30.00'], group: 'Flat'),
      'Hi Ben, a friendly reminder that you owe me €30.00 for Flat. '
      'Could you settle up when you get a chance? Thanks!',
    );
    expect(
      reminderText(name: 'Ben', amounts: ['€30.00']),
      isNot(contains(' for ')),
    );
  });

  testWidgets('connected: sends a reminder and confirms', (tester) async {
    final reminders = FakeReminders();
    await _pump(tester, _ben, AppMode.connected, reminders);
    await tester.tap(find.text('Remind'));
    await tester.pumpAndSettle();
    expect(reminders.sent, ['ben|g1']);
    expect(find.text('Reminder sent to Ben'), findsOneWidget);
  });

  testWidgets('connected: says when emails are turned off', (tester) async {
    final reminders = FakeReminders()..emailed = false;
    await _pump(tester, _ben, AppMode.connected, reminders);
    await tester.tap(find.text('Remind'));
    await tester.pumpAndSettle();
    expect(find.textContaining('turned reminder emails off'), findsOneWidget);
  });

  testWidgets('connected: shows the rate limit message', (tester) async {
    final reminders = FakeReminders()..limited = true;
    await _pump(tester, _ben, AppMode.connected, reminders);
    await tester.tap(find.text('Remind'));
    await tester.pumpAndSettle();
    expect(find.textContaining('in the last 24 hours'), findsOneWidget);
  });

  testWidgets('connected: people without email cannot be reminded', (
    tester,
  ) async {
    final reminders = FakeReminders();
    await _pump(tester, _sam, AppMode.connected, reminders);
    final button = tester.widget<OutlinedButton>(
      find.byKey(const ValueKey('remind-sam')),
    );
    expect(button.onPressed, isNull);
    final tooltip = tester.widget<Tooltip>(find.byType(Tooltip).first);
    expect(tooltip.message, contains('has no email address'));
    await tester.tap(find.text('Remind'), warnIfMissed: false);
    await tester.pumpAndSettle();
    expect(reminders.sent, isEmpty);
  });

  testWidgets('on this device: copies a reminder to send yourself', (
    tester,
  ) async {
    String? copied;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          copied = (call.arguments as Map)['text'] as String;
        }
        return null;
      },
    );
    addTearDown(
      () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      ),
    );
    final reminders = FakeReminders();
    // Placeholders are fine here: the user sends the text themselves.
    await _pump(tester, _sam, AppMode.local, reminders);
    await tester.tap(find.text('Share reminder'));
    await tester.pumpAndSettle();
    expect(reminders.sent, isEmpty);
    expect(copied, contains('you owe me €30.00 for Flat'));
    expect(find.textContaining('Reminder copied'), findsOneWidget);
  });
}
