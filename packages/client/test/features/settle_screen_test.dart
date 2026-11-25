import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';

import '../helpers.dart';

/// Fails the first payment after it may have reached the server.
class _Flaky extends LocalExpensesRepository {
  _Flaky(super.ledger);

  final ids = <String?>[];

  @override
  Future<Expense> recordPayment({
    String? id,
    String? groupId,
    required String fromUserId,
    required String toUserId,
    required int amount,
    required String currency,
    String? date,
    String? notes,
  }) async {
    ids.add(id);
    if (ids.length == 1) {
      throw const ApiError.network('Cannot reach the server.');
    }
    return super.recordPayment(
      id: id,
      groupId: groupId,
      fromUserId: fromUserId,
      toUserId: toUserId,
      amount: amount,
      currency: currency,
      date: date,
      notes: notes,
    );
  }
}

void main() {
  testWidgets('recording again after a failure keeps the same payment id', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 2400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    late _Flaky flaky;
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(MemoryLocalStore()),
          memoryDatabase(),
          expensesRepositoryProvider.overrideWith(
            (ref) => flaky = _Flaky(ref.watch(localLedgerProvider)),
          ),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await settle(tester);
    await tester.tap(find.text('Start'));
    await settle(tester);
    await tester.enterText(find.byKey(const Key('local-name')), 'Ada');
    await tester.tap(find.byKey(const Key('local-start')));
    await settle(tester);

    final container = ProviderScope.containerOf(
      tester.element(find.byType(SplitSpendsApp)),
    );
    final me = container.read(meProvider)!;
    final flat = await pumpFuture(
      tester,
      container
          .read(groupsRepositoryProvider)
          .create(
            const GroupInput(
              name: 'Flat',
              members: [MemberInput(name: 'Ben')],
            ),
          ),
    );
    final ben = flat.members.firstWhere((m) => m.id != me.id);
    unawaited(
      container
          .read(routerProvider)
          .push(
            Routes.settle,
            extra: SettleArgs(
              groupId: flat.id,
              fromUserId: ben.id,
              toUserId: me.id,
              amount: 1200,
              currency: 'USD',
            ),
          ),
    );
    await settle(tester);

    await tester.tap(find.text('Record payment'));
    await settle(tester);
    expect(flaky.ids, hasLength(1));
    await tester.tap(find.text('Record payment'));
    await settle(tester);

    expect(flaky.ids, hasLength(2));
    expect(flaky.ids.first, isNotNull);
    expect(flaky.ids.last, flaky.ids.first);
    final list = await pumpFuture(
      tester,
      container
          .read(expensesRepositoryProvider)
          .list(ExpenseQuery(groupId: flat.id)),
    );
    expect(list.items.single.id, flaky.ids.first);
  });
}
