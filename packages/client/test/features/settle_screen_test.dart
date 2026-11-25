import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/expenses/data/providers.dart';
import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

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

/// Records the currency of every payment.
class _Recording extends LocalExpensesRepository {
  _Recording(super.ledger);

  final currencies = <String>[];

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
  }) {
    currencies.add(currency);
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

/// Holds every payment until [release] completes, and counts list fetches.
class _Slow extends LocalExpensesRepository {
  _Slow(super.ledger);

  final release = Completer<void>();
  var lists = 0;

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
    await release.future;
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

  @override
  Future<PageResult<Expense>> list(
    ExpenseQuery query, {
    int limit = 50,
    int offset = 0,
  }) {
    lists++;
    return super.list(query, limit: limit, offset: offset);
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

  testWidgets('settle from a deep link uses the group currency', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 2400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    late _Recording recording;
    final loaded = Completer<Group>();
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(MemoryLocalStore()),
          memoryDatabase(),
          expensesRepositoryProvider.overrideWith(
            (ref) => recording = _Recording(ref.watch(localLedgerProvider)),
          ),
          groupProvider.overrideWith((ref, id) => loaded.future),
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
    expect(container.read(meProvider)!.defaultCurrency, isNot('EUR'));
    final paris = await pumpFuture(
      tester,
      container
          .read(groupsRepositoryProvider)
          .create(
            const GroupInput(
              name: 'Paris',
              defaultCurrency: 'EUR',
              members: [MemberInput(name: 'Ben')],
            ),
          ),
    );
    unawaited(
      container
          .read(routerProvider)
          .push('${Routes.settle}?groupId=${paris.id}'),
    );
    await tester.pump();
    await tester.pump();
    final record = find.widgetWithText(FilledButton, 'Record payment');
    expect(tester.widget<FilledButton>(record).onPressed, isNull);

    loaded.complete(paris);
    await settle(tester);
    await tester.tap(find.text('Paid to'));
    await settle(tester);
    await tester.tap(find.text('Ben').last);
    await settle(tester);
    await tester.enterText(
      find.descendant(
        of: find.byType(AmountField),
        matching: find.byType(TextField),
      ),
      '30',
    );
    await tester.tap(record);
    await settle(tester);

    expect(recording.currencies, ['EUR']);
  });

  testWidgets('closing the settle screen during save still refreshes', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 2400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    late _Slow slow;
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(MemoryLocalStore()),
          memoryDatabase(),
          expensesRepositoryProvider.overrideWith(
            (ref) => slow = _Slow(ref.watch(localLedgerProvider)),
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
    final query = ExpenseQuery(groupId: flat.id);
    final feed = container.listen(expensesFeedProvider(query), (_, _) {});
    addTearDown(feed.close);
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
            ),
          ),
    );
    await settle(tester);
    await tester.tap(find.text('Record payment'));
    await tester.pump();

    container.read(routerProvider).pop();
    await settle(tester);
    expect(find.text('Record payment'), findsNothing);
    final fetched = slow.lists;
    slow.release.complete();
    await settle(tester);

    expect(tester.takeException(), isNull);
    expect(slow.lists, greaterThan(fetched));
    final page = await pumpFuture(
      tester,
      container.read(expensesFeedProvider(query).future),
    );
    expect(page.items.single.isPayment, isTrue);
  });
}
