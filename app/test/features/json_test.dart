import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/features/activity/domain/activity.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/friends/domain/friend.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';

const _user = {
  'id': 'u1',
  'email': 'ada@example.com',
  'name': 'Ada Lovelace',
  'defaultCurrency': 'EUR',
  'avatarColor': '#7C4DFF',
  'status': 'active',
};

void main() {
  test('Group parses members and drops zero balances', () {
    final g = Group.fromJson({
      'id': 'g1',
      'name': 'Lisbon trip',
      'kind': 'trip',
      'defaultCurrency': 'EUR',
      'simplifyDebts': true,
      'emoji': null,
      'archived': false,
      'members': [_user],
      'balances': [
        {'currency': 'EUR', 'amount': 1250},
        {'currency': 'USD', 'amount': 0},
      ],
    });
    expect(g.kind, GroupKind.trip);
    expect(g.displayEmoji, '✈️');
    expect(g.members.single.initials, 'AL');
    expect(g.balances.single.amount, 1250);
  });

  test('Expense parses shares, items and the caller balance', () {
    final e = Expense.fromJson({
      'id': 'e1',
      'groupId': 'g1',
      'kind': 'expense',
      'description': 'Dinner',
      'category': 'food.dining',
      'currency': 'EUR',
      'total': 9000,
      'date': '2026-11-20',
      'notes': '',
      'splitMethod': 'itemized',
      'shares': [
        {'userId': 'u1', 'paid': 9000, 'owed': 3000, 'value': null},
        {'userId': 'u2', 'paid': 0, 'owed': 6000, 'value': null},
      ],
      'items': [
        {
          'name': 'Pizza',
          'amount': 2400,
          'userIds': ['u1'],
        },
      ],
      'tax': 100,
      'tip': 0,
      'receiptId': null,
      'recurrence': 'monthly',
      'deleted': false,
      'commentCount': 2,
      'myBalance': 6000,
    });
    expect(e.splitMethod, SplitMethod.itemized);
    expect(e.recurrence, Recurrence.monthly);
    expect(e.payers.single.userId, 'u1');
    expect(e.shareOf('u2')!.net, -6000);
    expect(e.items.single.userIds, ['u1']);
    expect(e.myBalance, 6000);
  });

  test('ExpenseInput serializes the split for each method', () {
    const equal = ExpenseInput(
      description: 'Taxi',
      currency: 'USD',
      total: 3000,
      date: '2026-11-24',
      payers: [Payer(userId: 'u1', amount: 3000)],
      splits: [
        SplitEntry(userId: 'u1'),
        SplitEntry(userId: 'u2'),
      ],
    );
    final json = equal.toJson();
    expect(json['splitMethod'], 'equal');
    expect(json['splits'], [
      {'userId': 'u1'},
      {'userId': 'u2'},
    ]);
    expect(json.containsKey('items'), isFalse);

    const itemized = ExpenseInput(
      description: 'Lunch',
      currency: 'USD',
      total: 1100,
      date: '2026-11-24',
      splitMethod: SplitMethod.itemized,
      payers: [Payer(userId: 'u1', amount: 1100)],
      items: [
        LineItem(name: 'Soup', amount: 1000, userIds: ['u2']),
      ],
      tax: 100,
    );
    final j2 = itemized.toJson();
    expect(j2['items'], [
      {
        'name': 'Soup',
        'amount': 1000,
        'userIds': ['u2'],
      },
    ]);
    expect(j2['tax'], 100);
    expect(j2.containsKey('splits'), isFalse);
  });

  test('ExpenseDraft parses with warnings', () {
    final d = ExpenseInput.fromJson({
      'groupId': null,
      'description': 'Dinner',
      'currency': 'EUR',
      'total': 6000,
      'date': '2026-11-24',
      'category': 'food.dining',
      'splitMethod': 'equal',
      'payers': [
        {'userId': 'u1', 'amount': 6000},
      ],
      'splits': [
        {'userId': 'u1'},
      ],
      'warnings': ['Could not match Sam'],
    });
    expect(d.total, 6000);
    expect(d.warnings, ['Could not match Sam']);
  });

  test('Config, friends, activity and scans parse', () {
    final config = AppConfig.fromJson({
      'appName': 'SplitSpends',
      'aiEnabled': true,
      'signupMode': 'invite',
      'currencies': [
        {'code': 'JPY', 'name': 'Yen', 'symbol': '¥', 'exponent': 0},
      ],
      'categories': [
        {
          'id': 'food.dining',
          'name': 'Dining out',
          'group': 'Food',
          'icon': 'restaurant',
        },
      ],
    });
    expect(config.currency('JPY').exponent, 0);
    expect(config.currency('KWD').exponent, 3);
    expect(config.category('food.dining')!.name, 'Dining out');

    final friend = FriendDetail.fromJson({
      'user': _user,
      'balances': [
        {'currency': 'EUR', 'amount': -500},
      ],
      'groups': [
        {
          'groupId': 'g1',
          'groupName': 'Home',
          'balances': [
            {'currency': 'EUR', 'amount': -500},
          ],
        },
      ],
      'nonGroup': <Object>[],
    });
    expect(friend.groups.single.groupName, 'Home');

    final activity = Activity.fromJson({
      'id': 'a1',
      'kind': 'expense.created',
      'actor': _user,
      'groupId': 'g1',
      'expenseId': 'e1',
      'summary': 'Ada added Dinner',
      'data': <String, Object>{},
      'createdAt': '2026-11-24T10:00:00Z',
    });
    expect(activity.actor.firstName, 'Ada');

    final scan = ReceiptScan.fromJson({
      'merchant': 'Cafe',
      'date': null,
      'currency': 'EUR',
      'items': [
        {'name': 'Pizza', 'quantity': 1, 'amount': 1200},
      ],
      'subtotal': 1200,
      'tax': 0,
      'tip': 0,
      'total': 1200,
      'category': 'food.dining',
    });
    expect(scan.items.single.amount, 1200);

    final summary = BalanceSummary.fromJson({
      'balances': <Object>[],
      'owed': <Object>[],
      'owing': <Object>[],
    });
    expect(summary.isSettled, isTrue);
  });

  test('ApiError reads the error envelope', () {
    final e = ApiError.fromJson(422, {
      'error': 'Percentages must add up to 100.',
      'code': 'VALIDATION_ERROR',
      'field': 'splits',
    });
    expect(e.field, 'splits');
    expect(describeError(e), 'Percentages must add up to 100.');
    expect(ApiError.fromJson(503, {'error': 'off'}).isAiDisabled, isTrue);
  });
}
