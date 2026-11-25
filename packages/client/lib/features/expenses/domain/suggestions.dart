import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/features/friends/domain/friend.dart';

/// Payments that would settle up the signed-in user [meId]: the group
/// [debts] they take part in, then one per currency they share with each of
/// [friends] (only the friend [friendId] when given).
List<Debt> suggestions({
  required String meId,
  List<Debt> debts = const [],
  List<Friend> friends = const [],
  String? friendId,
}) => [
  for (final d in debts)
    if (d.from == meId || d.to == meId) d,
  for (final f in friends)
    if (friendId == null || f.user.id == friendId)
      for (final b in f.balances) _debt(meId, f.user.id, b),
];

/// A friend balance as the payment that clears it. A positive balance means
/// the friend owes you, see [Friend.balances].
Debt _debt(String meId, String friendId, Balance balance) => Debt(
  from: balance.amount > 0 ? friendId : meId,
  to: balance.amount > 0 ? meId : friendId,
  currency: balance.currency,
  amount: balance.amount.abs(),
);
