import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/widgets/states.dart';

/// A button that runs [onTap] with its own context.
Future<void> _pumpButton(
  WidgetTester tester,
  void Function(BuildContext context) onTap,
) async {
  await tester.pumpWidget(
    ProviderScope(
      child: MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => TextButton(
              onPressed: () => onTap(context),
              child: const Text('Go'),
            ),
          ),
        ),
      ),
    ),
  );
}

void main() {
  group('confirm', () {
    Future<Future<bool>> open(WidgetTester tester) async {
      late Future<bool> answer;
      await _pumpButton(
        tester,
        (context) => answer = confirm(
          context,
          title: 'Delete group?',
          message: 'This cannot be undone.',
          action: 'Delete',
          actionKey: const Key('confirm-delete'),
        ),
      );
      await tester.tap(find.text('Go'));
      await tester.pumpAndSettle();
      expect(find.text('Delete group?'), findsOneWidget);
      expect(find.text('This cannot be undone.'), findsOneWidget);
      return answer;
    }

    testWidgets('is true when the person takes the action', (tester) async {
      final answer = await open(tester);
      await tester.tap(find.byKey(const Key('confirm-delete')));
      await tester.pumpAndSettle();
      expect(await answer, isTrue);
    });

    testWidgets('is false when the person cancels', (tester) async {
      final answer = await open(tester);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(await answer, isFalse);
    });

    testWidgets('is false when the person dismisses the dialog', (
      tester,
    ) async {
      final answer = await open(tester);
      await tester.tapAt(Offset.zero);
      await tester.pumpAndSettle();
      expect(find.text('Delete group?'), findsNothing);
      expect(await answer, isFalse);
    });
  });

  group('runChange', () {
    testWidgets('says the change is done', (tester) async {
      late Future<bool> result;
      await _pumpButton(
        tester,
        (context) => result = runChange(context, (_) async {}, done: 'Saved'),
      );
      await tester.tap(find.text('Go'));
      await tester.pump();
      expect(await result, isTrue);
      expect(find.text('Saved'), findsOneWidget);
    });

    testWidgets('shows what went wrong', (tester) async {
      late Future<bool> result;
      await _pumpButton(
        tester,
        (context) => result = runChange(
          context,
          (_) async => throw const ApiError(
            status: 409,
            code: 'CONFLICT',
            message: 'Settle up first.',
          ),
          done: 'Saved',
        ),
      );
      await tester.tap(find.text('Go'));
      await tester.pump();
      expect(await result, isFalse);
      expect(find.text('Settle up first.'), findsOneWidget);
      expect(find.text('Saved'), findsNothing);
    });

    testWidgets('finishes the change after the screen is gone', (tester) async {
      final release = Completer<void>();
      ProviderContainer? used;
      late Future<bool> result;
      await _pumpButton(
        tester,
        (context) => result = runChange(context, (container) async {
          await release.future;
          used = container;
        }, done: 'Saved'),
      );
      final container = ProviderScope.containerOf(
        tester.element(find.text('Go')),
      );
      await tester.tap(find.text('Go'));
      await tester.pump();
      // The screen goes away while the change is still running.
      await tester.pumpWidget(
        UncontrolledProviderScope(
          container: container,
          child: const MaterialApp(home: SizedBox()),
        ),
      );
      release.complete();
      expect(await result, isTrue);
      expect(used, same(container));
    });
  });
}
