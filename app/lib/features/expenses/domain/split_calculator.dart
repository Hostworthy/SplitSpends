import 'package:splitspends/features/expenses/domain/expense.dart';

/// Outcome of a client-side split preview.
///
/// The server is authoritative; this mirrors its rules so the editor can show
/// live per-person amounts and validation before saving.
class SplitPreview {
  const SplitPreview({required this.owed, this.error, this.remaining = 0});

  final Map<String, int> owed;

  /// Human readable validation problem, null when valid.
  final String? error;

  /// For `exact`: minor units still unassigned (negative when over).
  /// For `percent`: basis points left until 100% (negative when over).
  final int remaining;

  bool get isValid => error == null;
}

/// Splits [amount] by integer [weights] with the largest-remainder method.
/// Leftover minor units go to the largest fractional parts, ties by order.
List<int> allocate(int amount, List<int> weights) {
  if (weights.isEmpty) return const [];
  final sum = weights.fold<int>(0, (a, b) => a + b);
  if (sum <= 0) return List.filled(weights.length, 0);
  final negative = amount < 0;
  final abs = amount.abs();
  final parts = <int>[];
  final remainders = <int>[];
  var assigned = 0;
  for (final w in weights) {
    final product = abs * w;
    parts.add(product ~/ sum);
    remainders.add(product % sum);
    assigned += product ~/ sum;
  }
  var left = abs - assigned;
  final order = List<int>.generate(weights.length, (i) => i)
    ..sort((a, b) {
      final byRemainder = remainders[b].compareTo(remainders[a]);
      return byRemainder != 0 ? byRemainder : a.compareTo(b);
    });
  for (final i in order) {
    if (left == 0) break;
    if (weights[i] == 0) continue;
    parts[i] += 1;
    left -= 1;
  }
  return negative ? parts.map((p) => -p).toList() : parts;
}

/// Computes what each participant owes for the given split.
///
/// [values] holds the per-user input for the method: minor units for
/// `exact` and `adjustment`, percentages for `percent`, share counts for
/// `shares`. Ignored for `equal` and `itemized`.
SplitPreview previewSplit({
  required SplitMethod method,
  required int total,
  required List<String> participants,
  Map<String, num> values = const {},
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
  switch (method) {
    case SplitMethod.equal:
      final parts = allocate(total, List.filled(participants.length, 1));
      return SplitPreview(owed: _zip(participants, parts));

    case SplitMethod.exact:
      final owed = {for (final p in participants) p: (values[p] ?? 0).round()};
      final sum = owed.values.fold<int>(0, (a, b) => a + b);
      final remaining = total - sum;
      if (owed.values.any((v) => v < 0)) {
        return SplitPreview(owed: owed, error: 'Amounts cannot be negative');
      }
      return SplitPreview(
        owed: owed,
        remaining: remaining,
        error: remaining == 0
            ? null
            : remaining > 0
            ? 'Amounts are short of the total'
            : 'Amounts exceed the total',
      );

    case SplitMethod.percent:
      final points = [
        for (final p in participants) ((values[p] ?? 0) * 100).round(),
      ];
      if (points.any((v) => v < 0)) {
        return const SplitPreview(
          owed: {},
          error: 'Percentages cannot be negative',
        );
      }
      final sum = points.fold<int>(0, (a, b) => a + b);
      final owed = _zip(participants, allocate(total, points));
      return SplitPreview(
        owed: owed,
        remaining: 10000 - sum,
        error: sum == 10000 ? null : 'Percentages must add up to 100%',
      );

    case SplitMethod.shares:
      final weights = [
        for (final p in participants) ((values[p] ?? 0) * 1000).round(),
      ];
      if (weights.any((v) => v < 0)) {
        return const SplitPreview(owed: {}, error: 'Shares cannot be negative');
      }
      if (weights.every((w) => w == 0)) {
        return const SplitPreview(owed: {}, error: 'Give someone a share');
      }
      return SplitPreview(owed: _zip(participants, allocate(total, weights)));

    case SplitMethod.adjustment:
      final adjustments = [
        for (final p in participants) (values[p] ?? 0).round(),
      ];
      final adjusted = adjustments.fold<int>(0, (a, b) => a + b);
      final rest = total - adjusted;
      if (rest < 0) {
        return const SplitPreview(
          owed: {},
          error: 'Adjustments exceed the total',
        );
      }
      final even = allocate(rest, List.filled(participants.length, 1));
      final owed = <String, int>{};
      for (var i = 0; i < participants.length; i++) {
        owed[participants[i]] = even[i] + adjustments[i];
      }
      if (owed.values.any((v) => v < 0)) {
        return SplitPreview(owed: owed, error: 'Someone would owe less than 0');
      }
      return SplitPreview(owed: owed);

    case SplitMethod.itemized:
      return _itemized(total, items, tax, tip);
  }
}

SplitPreview _itemized(int total, List<LineItem> items, int tax, int tip) {
  if (items.isEmpty) {
    return const SplitPreview(owed: {}, error: 'Add at least one item');
  }
  final subtotals = <String, int>{};
  for (final item in items) {
    if (item.userIds.isEmpty) {
      return SplitPreview(
        owed: subtotals,
        error: 'Assign “${item.name.isEmpty ? 'item' : item.name}” to someone',
      );
    }
    final parts = allocate(item.amount, List.filled(item.userIds.length, 1));
    for (var i = 0; i < item.userIds.length; i++) {
      subtotals.update(
        item.userIds[i],
        (v) => v + parts[i],
        ifAbsent: () => parts[i],
      );
    }
  }
  final users = subtotals.keys.toList();
  final extras = allocate(tax + tip, [for (final u in users) subtotals[u]!]);
  final owed = <String, int>{};
  for (var i = 0; i < users.length; i++) {
    owed[users[i]] = subtotals[users[i]]! + extras[i];
  }
  final itemsTotal = items.fold<int>(0, (a, i) => a + i.amount);
  final expected = itemsTotal + tax + tip;
  return SplitPreview(
    owed: owed,
    remaining: total - expected,
    error: expected == total ? null : 'Items, tax and tip must equal the total',
  );
}

/// Validates payers: they must add up to the total.
String? validatePayers(int total, List<Payer> payers) {
  if (payers.isEmpty) return 'Choose who paid';
  if (payers.any((p) => p.amount < 0)) return 'Paid amounts cannot be negative';
  final sum = payers.fold<int>(0, (a, p) => a + p.amount);
  if (sum != total) return 'Paid amounts must add up to the total';
  return null;
}

Map<String, int> _zip(List<String> keys, List<int> values) => {
  for (var i = 0; i < keys.length; i++) keys[i]: values[i],
};
