import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/features/groups/presentation/emoji_picker.dart';

void main() {
  test('the catalog is large and has no duplicates within a category', () {
    final all = groupEmojis.values.expand((e) => e).toList();
    expect(all.length, greaterThan(150));
    for (final entry in groupEmojis.entries) {
      expect(entry.value.toSet().length, entry.value.length, reason: entry.key);
    }
    expect(quickEmojis.every(all.contains), isTrue);
  });

  Future<String?> open(WidgetTester tester, Future<void> Function() act) async {
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    String? picked;
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (context) => TextButton(
            onPressed: () async => picked = await pickEmoji(context),
            child: const Text('open'),
          ),
        ),
      ),
    );
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    await act();
    await tester.pumpAndSettle();
    return picked;
  }

  testWidgets('tapping an icon picks it', (tester) async {
    final picked = await open(tester, () async {
      await tester.ensureVisible(find.text('🍕'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('🍕'));
    });
    expect(picked, '🍕');
  });

  testWidgets('any typed emoji can be used, plain text cannot', (tester) async {
    final picked = await open(tester, () async {
      await tester.enterText(find.byKey(const Key('emoji-custom')), 'abc');
      await tester.pump();
      expect(find.byIcon(Icons.check_rounded), findsNothing);
      await tester.enterText(find.byKey(const Key('emoji-custom')), '🦖 dino');
      await tester.pump();
      await tester.tap(find.byIcon(Icons.check_rounded));
    });
    expect(picked, '🦖');
  });
}
