import 'package:splitspends/core/ledger/rational.dart';

/// Thrown when every weight is zero.
class NoWeightsError implements Exception {
  const NoWeightsError();
}

/// Splits [total] minor units in proportion to [weights] so the parts sum to
/// exactly [total]. Mirrors `money.Allocate` in the backend: floor every
/// exact share, then hand out the leftover units one at a time to the largest
/// fractional remainders; ties keep the input order (a stable sort).
List<int> allocate(int total, List<Rational> weights) {
  var sum = Rational.zero;
  for (final w in weights) {
    if (w.sign < 0) throw ArgumentError('negative weight');
    sum = sum + w;
  }
  if (sum.sign == 0) throw const NoWeightsError();

  final negative = total < 0;
  final abs = Rational.integer(negative ? -total : total);
  final parts = List<int>.filled(weights.length, 0);
  final remainders = <({int index, Rational frac})>[];
  var assigned = 0;
  for (var i = 0; i < weights.length; i++) {
    final exact = abs * weights[i] / sum;
    final floor = exact.truncate();
    parts[i] = floor.toInt();
    assigned += parts[i];
    remainders.add((index: i, frac: exact - Rational(floor)));
  }
  // List.sort is not stable; break ties by index to match Go's SliceStable.
  remainders.sort((a, b) {
    final byFrac = b.frac.compareTo(a.frac);
    return byFrac != 0 ? byFrac : a.index.compareTo(b.index);
  });
  var left = (negative ? -total : total) - assigned;
  for (var k = 0; left > 0; left--, k++) {
    parts[remainders[k % remainders.length].index]++;
  }
  if (negative) {
    for (var i = 0; i < parts.length; i++) {
      parts[i] = -parts[i];
    }
  }
  return parts;
}

/// Splits [total] into [n] near-equal parts; earlier parts get the extra units.
List<int> even(int total, int n) =>
    n == 0 ? const [] : allocate(total, List.filled(n, Rational.integer(1)));
