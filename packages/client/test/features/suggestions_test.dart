import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/expenses/domain/suggestions.dart';
import 'package:splitspends/features/friends/domain/friend.dart';

/// Debts as "from > to currency amount", for readable expectations.
List<String> _show(List<Debt> debts) => [
  for (final d in debts) '${d.from} > ${d.to} ${d.currency} ${d.amount}',
];

Friend _friend(String id, List<Balance> balances) =>
    Friend(user: User(id: id, name: id), balances: balances);

void main() {
  final friends = [
    _friend('ben', const [Balance(currency: 'EUR', amount: 1200)]),
    _friend('cy', const [
      Balance(currency: 'EUR', amount: -500),
      Balance(currency: 'USD', amount: 300),
    ]),
  ];

  for (final (name, friendId, want) in [
    (
      'friend owes me and I owe friend',
      null,
      ['ben > me EUR 1200', 'me > cy EUR 500', 'cy > me USD 300'],
    ),
    ('filtered to one friend', 'cy', ['me > cy EUR 500', 'cy > me USD 300']),
    ('filtered to a friend with no balance', 'dee', <String>[]),
  ]) {
    test(name, () {
      expect(
        _show(suggestions(meId: 'me', friends: friends, friendId: friendId)),
        want,
      );
    });
  }

  test('keeps only the group debts I take part in', () {
    const debts = [
      Debt(from: 'me', to: 'ben', currency: 'EUR', amount: 700),
      Debt(from: 'cy', to: 'ben', currency: 'EUR', amount: 400),
      Debt(from: 'cy', to: 'me', currency: 'EUR', amount: 100),
    ];
    expect(_show(suggestions(meId: 'me', debts: debts)), [
      'me > ben EUR 700',
      'cy > me EUR 100',
    ]);
  });
}
