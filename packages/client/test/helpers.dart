import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/storage/local_db.dart';

/// A fresh in-memory database for one widget test. Sembast's cooperator
/// yields with timers, which never fire under the widget tester's fake
/// clock, so it is turned off.
Override memoryDatabase() {
  disableSembastCooperator();
  return databaseProvider.overrideWith(
    (ref) => databaseFactoryMemory.openDatabase(
      'test-${DateTime.now().microsecondsSinceEpoch}',
    ),
  );
}

/// pumpAndSettle that fails fast (5 s of fake time) instead of hanging on
/// an endless animation.
Future<void> settle(WidgetTester tester) => tester.pumpAndSettle(
  const Duration(milliseconds: 100),
  EnginePhase.sendSemanticsUpdate,
  const Duration(seconds: 5),
);

/// Completes [future] under the widget tester's fake clock by pumping.
Future<T> pumpFuture<T>(WidgetTester tester, Future<T> future) async {
  T? value;
  Object? error;
  var done = false;
  unawaited(
    future.then(
      (v) {
        value = v;
        done = true;
      },
      onError: (Object e) {
        error = e;
        done = true;
      },
    ),
  );
  for (var i = 0; i < 200 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 20));
  }
  if (!done) throw StateError('Future did not complete');
  if (error != null) Error.throwWithStackTrace(error!, StackTrace.current);
  return value as T;
}
