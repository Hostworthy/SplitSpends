import 'package:splitspends_ledger/ledger.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

/// Live preview of a split for the editor, computed with the same rules as
/// the server (lib/core/ledger, a port of the backend). The server stays
/// authoritative; this only drives validation and per-person amounts.
class SplitPreview {
  const SplitPreview({required this.owed, this.error, this.remaining = 0});

  final Map<String, int> owed;

  /// Human readable problem, null when valid.
  final String? error;

  /// For `exact`: minor units still unassigned (negative when over).
  /// For `percent`: ten-thousandths of a percent left until 100% (negative
  /// when over), see [tenThousandths].
  final int remaining;

  bool get isValid => error == null;
}

/// Computes what each participant owes.
///
/// [values] holds the per-user decimal text for the method: minor units for
/// `exact` and `adjustment`, percentages for `percent`, share counts for
/// `shares`. Ignored for `equal` and `itemized`.
SplitPreview previewSplit({
  required SplitMethod method,
  required int total,
  required List<String> participants,
  Map<String, String> values = const {},
  List<LineItem> items = const [],
  int tax = 0,
  int tip = 0,
}) {
  if (total <= 0) {
    return const SplitPreview(owed: {}, error: 'Enter an amount');
  }
  if (method != SplitMethod.itemized && participants.isEmpty) {
    return const SplitPreview(owed: {}, error: 'Choose who is splitting');
  }
  final remaining = _remaining(method, total, participants, values);
  final splitValues = [
    for (final id in participants)
      SplitValueInput(id, switch (method) {
        SplitMethod.equal || SplitMethod.itemized => null,
        SplitMethod.adjustment => values[id],
        _ => values[id] ?? '0',
      }),
  ];
  try {
    final n = normalize(
      ExpenseRequest(
        description: 'preview',
        currency: 'XXX',
        total: total,
        method: method.wire,
        // A synthetic payer keeps payer rules out of the split preview.
        payers: [PayerInput(_previewPayer, total)],
        splits: splitValues,
        items: [for (final i in items) ItemInput(i.name, i.amount, i.userIds)],
        tax: tax,
        tip: tip,
      ),
      today: DateTime.utc(2000),
    );
    return SplitPreview(
      owed: {
        for (final s in n.shares)
          if (s.userId != _previewPayer) s.userId: s.owed,
      },
      remaining: remaining,
    );
  } on LedgerError catch (error) {
    return SplitPreview(
      owed: const {},
      remaining: remaining,
      error: _friendly(error, method),
    );
  }
}

const _previewPayer = 'preview-payer';

int _remaining(
  SplitMethod method,
  int total,
  List<String> participants,
  Map<String, String> values,
) {
  Rational sum = Rational.zero;
  for (final id in participants) {
    sum = sum + (Rational.tryParseDecimal(values[id] ?? '') ?? Rational.zero);
  }
  return switch (method) {
    SplitMethod.exact => total - sum.truncate().toInt(),
    SplitMethod.percent => tenThousandths(Rational.integer(100) - sum),
    _ => 0,
  };
}

/// [percent] in ten-thousandths of a percent, the finest step a percentage
/// may have. A finer value, which the ledger refuses, is rounded away from
/// zero, so a gap never reads as none.
int tenThousandths(Rational percent) {
  final scaled = percent * Rational.integer(10000);
  final whole = scaled.truncate();
  return (scaled.isInteger ? whole : whole + BigInt.from(scaled.sign)).toInt();
}

/// "40% left", "10% over" or "100% assigned", for [units] ten-thousandths
/// of a percent left.
String percentLeft(int units) => units == 0
    ? '100% assigned'
    : '${percentText(units.abs())}% ${units > 0 ? 'left' : 'over'}';

/// `123456` (units of 0.0001%) as `12.3456`, without trailing zeros.
String percentText(int units) {
  final whole = units ~/ 10000;
  var fraction = (units % 10000).toString().padLeft(4, '0');
  fraction = fraction.replaceFirst(RegExp(r'0+$'), '');
  return fraction.isEmpty ? '$whole' : '$whole.$fraction';
}

String _friendly(LedgerError error, SplitMethod method) {
  if (error.field == 'total' && method == SplitMethod.itemized) {
    return 'Items, tax and tip must equal the total';
  }
  if (error.field == 'items' && error.message.startsWith('Assign')) {
    return 'Assign every item to someone';
  }
  return error.message.endsWith('.')
      ? error.message.substring(0, error.message.length - 1)
      : error.message;
}

/// Validates payers: they must add up to the total.
String? validatePayers(int total, List<Payer> payers) {
  if (payers.isEmpty) return 'Choose who paid';
  if (payers.any((p) => p.amount < 0)) return 'Paid amounts cannot be negative';
  final sum = payers.fold<int>(0, (a, p) => a + p.amount);
  if (sum != total) return 'Paid amounts must add up to the total';
  return null;
}
