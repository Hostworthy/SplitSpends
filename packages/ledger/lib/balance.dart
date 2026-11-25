// Dart port of packages/core/domains/balance/ledger.go and the
// sheet logic of service.go. spec/vectors.json runs against both.

/// Per-currency amounts. Positive means owed to you.
typedef Balances = Map<String, int>;

extension BalancesOps on Balances {
  void addAmount(String currency, int amount) =>
      this[currency] = (this[currency] ?? 0) + amount;

  bool get settled => values.every((a) => a == 0);

  /// Non-zero entries sorted by currency.
  List<MapEntry<String, int>> listed() =>
      entries.where((e) => e.value != 0).toList()
        ..sort((a, b) => a.key.compareTo(b.key));
}

class LedgerShare {
  const LedgerShare(this.userId, this.paid, this.owed);

  final String userId;
  final int paid;
  final int owed;
}

/// A non-deleted expense or payment reduced to what balances need.
class LedgerEntry {
  const LedgerEntry({
    this.groupId,
    required this.currency,
    required this.shares,
  });

  final String? groupId;
  final String currency;
  final List<LedgerShare> shares;
}

/// [from] owes [to] [amount] in [currency].
class LedgerDebt {
  const LedgerDebt(this.from, this.to, this.currency, this.amount);

  final String from;
  final String to;
  final String currency;
  final int amount;

  @override
  bool operator ==(Object other) =>
      other is LedgerDebt &&
      other.from == from &&
      other.to == to &&
      other.currency == currency &&
      other.amount == amount;

  @override
  int get hashCode => Object.hash(from, to, currency, amount);

  @override
  String toString() => '$from→$to $amount $currency';
}

/// Each user's paid minus owed per currency.
Map<String, Balances> nets(Iterable<LedgerEntry> entries) {
  final out = <String, Balances>{};
  for (final e in entries) {
    for (final s in e.shares) {
      out
          .putIfAbsent(s.userId, () => {})
          .addAmount(e.currency, s.paid - s.owed);
    }
  }
  return out;
}

/// Who owes whom without simplification: each expense's debtors owe its
/// creditors in proportion to what each creditor fronted; opposite debts
/// between the same two people cancel out. Within an expense each debtor's
/// debts add up to what they owe and each creditor is owed exactly what they
/// fronted, so the debts always agree with [nets].
List<LedgerDebt> pairwise(Iterable<LedgerEntry> entries) {
  final pairs = <(String, String, String), int>{};
  for (final e in entries) {
    final shares = [...e.shares]..sort((a, b) => a.userId.compareTo(b.userId));
    final creditors = <String>[], debtors = <String>[];
    final credits = <int>[], debts = <int>[];
    for (final s in shares) {
      final net = s.paid - s.owed;
      if (net > 0) {
        creditors.add(s.userId);
        credits.add(net);
      }
      if (net < 0) {
        debtors.add(s.userId);
        debts.add(-net);
      }
    }
    if (creditors.isEmpty) continue;
    final cells = _spread(debts, credits);
    for (var i = 0; i < debtors.length; i++) {
      for (var j = 0; j < creditors.length; j++) {
        _addPair(pairs, debtors[i], creditors[j], e.currency, cells[i][j]);
      }
    }
  }
  final out = <LedgerDebt>[];
  pairs.forEach((k, amount) {
    if (amount > 0) out.add(LedgerDebt(k.$1, k.$2, k.$3, amount));
    if (amount < 0) out.add(LedgerDebt(k.$2, k.$1, k.$3, -amount));
  });
  sortDebts(out);
  return out;
}

/// Splits [debts] among creditors in proportion to [credits] so that row i
/// adds up to `debts[i]` and column j to `credits[j]`, like `spread` in Go:
/// floor every exact share, then hand out the leftover units from the first
/// row and column onwards. Callers pass both lists in user id order.
List<List<int>> _spread(List<int> debts, List<int> credits) {
  var total = BigInt.zero;
  for (final c in credits) {
    total += BigInt.from(c);
  }
  final rows = [...debts], cols = [...credits];
  final cells = [
    for (var i = 0; i < debts.length; i++)
      [
        for (var j = 0; j < credits.length; j++)
          (BigInt.from(debts[i]) * BigInt.from(credits[j]) ~/ total).toInt(),
      ],
  ];
  for (var i = 0; i < debts.length; i++) {
    for (var j = 0; j < credits.length; j++) {
      rows[i] -= cells[i][j];
      cols[j] -= cells[i][j];
    }
  }
  for (var i = 0, j = 0; i < rows.length && j < cols.length;) {
    final n = rows[i] < cols[j] ? rows[i] : cols[j];
    cells[i][j] += n;
    rows[i] -= n;
    cols[j] -= n;
    if (rows[i] == 0) i++;
    if (cols[j] == 0) j++;
  }
  return cells;
}

void _addPair(
  Map<(String, String, String), int> pairs,
  String from,
  String to,
  String currency,
  int amount,
) {
  if (amount == 0 || from == to) return;
  if (from.compareTo(to) < 0) {
    pairs[(from, to, currency)] = (pairs[(from, to, currency)] ?? 0) + amount;
  } else {
    pairs[(to, from, currency)] = (pairs[(to, from, currency)] ?? 0) - amount;
  }
}

/// Fewest practical transfers that settle [netsByUser]: per currency the
/// largest debtor repeatedly pays the largest creditor.
List<LedgerDebt> simplify(Map<String, Balances> netsByUser) {
  final byCurrency = <String, List<({String id, int amount})>>{};
  netsByUser.forEach((id, b) {
    b.forEach((c, a) {
      if (a != 0) byCurrency.putIfAbsent(c, () => []).add((id: id, amount: a));
    });
  });
  final out = <LedgerDebt>[];
  int order(({String id, int amount}) a, ({String id, int amount}) b) =>
      a.amount != b.amount
      ? b.amount.compareTo(a.amount)
      : a.id.compareTo(b.id);
  byCurrency.forEach((currency, list) {
    final creditors = [
      for (final p in list)
        if (p.amount > 0) (id: p.id, amount: p.amount),
    ];
    final debtors = [
      for (final p in list)
        if (p.amount < 0) (id: p.id, amount: -p.amount),
    ];
    while (creditors.isNotEmpty && debtors.isNotEmpty) {
      creditors.sort(order);
      debtors.sort(order);
      final c = creditors.first, d = debtors.first;
      final amount = c.amount < d.amount ? c.amount : d.amount;
      out.add(LedgerDebt(d.id, c.id, currency, amount));
      creditors[0] = (id: c.id, amount: c.amount - amount);
      debtors[0] = (id: d.id, amount: d.amount - amount);
      if (creditors[0].amount == 0) creditors.removeAt(0);
      if (debtors[0].amount == 0) debtors.removeAt(0);
    }
  });
  sortDebts(out);
  return out;
}

List<LedgerDebt> debts(
  Iterable<LedgerEntry> entries, {
  required bool simplified,
}) => simplified ? simplify(nets(entries)) : pairwise(entries);

/// Currency, then amount descending, then from, then to.
void sortDebts(List<LedgerDebt> ds) => ds.sort((a, b) {
  if (a.currency != b.currency) return a.currency.compareTo(b.currency);
  if (a.amount != b.amount) return b.amount.compareTo(a.amount);
  if (a.from != b.from) return a.from.compareTo(b.from);
  return a.to.compareTo(b.to);
});

/// The slice of a group that balances need.
class LedgerGroup {
  const LedgerGroup(this.id, this.name, {required this.simplified});

  final String id;
  final String name;
  final bool simplified;
}

/// The balance between the caller and one other person.
class FriendView {
  final Balances total = {};
  final Map<String, Balances> byGroup = {};
  final Balances nonGroup = {};
}

/// Everything the caller owes and is owed (balance.Service.For).
class Sheet {
  final Map<String, FriendView> friends = {};
  final Map<String, Balances> groups = {};

  FriendView friend(String id) => friends.putIfAbsent(id, FriendView.new);

  /// Overall net, what others owe you, and what you owe (positive amounts).
  ({Balances net, Balances owed, Balances owing}) totals() {
    final Balances net = {}, owed = {}, owing = {};
    for (final f in friends.values) {
      f.total.forEach((c, a) {
        net.addAmount(c, a);
        if (a > 0) owed.addAmount(c, a);
        if (a < 0) owing.addAmount(c, -a);
      });
    }
    return (net: net, owed: owed, owing: owing);
  }
}

/// Computes [me]'s sheet: per-group debts honouring each group's
/// simplification setting, plus pairwise debts of non-group entries.
Sheet sheetFor(
  String me, {
  required List<LedgerGroup> groups,
  required Map<String, List<LedgerEntry>> groupEntries,
  required List<LedgerEntry> nonGroup,
}) {
  final sheet = Sheet();
  void collect(
    List<LedgerDebt> ds,
    void Function(String other, String currency, int amount) apply,
  ) {
    for (final d in ds) {
      if (d.to == me) {
        apply(d.from, d.currency, d.amount);
      } else if (d.from == me) {
        apply(d.to, d.currency, -d.amount);
      }
    }
  }

  for (final g in groups) {
    final list = groupEntries[g.id] ?? const <LedgerEntry>[];
    sheet.groups[g.id] = nets(list)[me] ?? {};
    collect(debts(list, simplified: g.simplified), (other, currency, amount) {
      final f = sheet.friend(other);
      f.byGroup.putIfAbsent(g.id, () => {}).addAmount(currency, amount);
      f.total.addAmount(currency, amount);
    });
  }
  collect(pairwise(nonGroup), (other, currency, amount) {
    final f = sheet.friend(other);
    f.nonGroup.addAmount(currency, amount);
    f.total.addAmount(currency, amount);
  });
  return sheet;
}
