import 'package:splitspends/core/models/json.dart';

/// An amount in one currency. Positive means you are owed.
class Balance {
  const Balance({required this.currency, required this.amount});

  factory Balance.fromJson(JsonMap json) => Balance(
    currency: json['currency'] as String,
    amount: (json['amount'] as num).toInt(),
  );

  final String currency;
  final int amount;

  bool get isZero => amount == 0;

  JsonMap toJson() => {'currency': currency, 'amount': amount};
}

List<Balance> parseBalances(Object? value) => (value as List? ?? const [])
    .cast<JsonMap>()
    .map(Balance.fromJson)
    .where((b) => !b.isZero)
    .toList();

/// An edge of the settle-up graph: [from] owes [to] [amount].
class Debt {
  const Debt({
    required this.from,
    required this.to,
    required this.currency,
    required this.amount,
  });

  factory Debt.fromJson(JsonMap json) => Debt(
    from: json['from'] as String,
    to: json['to'] as String,
    currency: json['currency'] as String,
    amount: (json['amount'] as num).toInt(),
  );

  final String from;
  final String to;
  final String currency;
  final int amount;
}

/// Summary of balances: net, and the owed / owing sides separately.
class BalanceSummary {
  const BalanceSummary({
    this.balances = const [],
    this.owed = const [],
    this.owing = const [],
  });

  factory BalanceSummary.fromJson(JsonMap json) => BalanceSummary(
    balances: parseBalances(json['balances']),
    owed: parseBalances(json['owed']),
    owing: parseBalances(json['owing']),
  );

  final List<Balance> balances;
  final List<Balance> owed;
  final List<Balance> owing;

  bool get isSettled => balances.isEmpty;
}
