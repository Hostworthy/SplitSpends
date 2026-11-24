import 'package:splitspends_ledger/rational.dart';
import 'package:splitspends_ledger/split.dart';

// Dart port of packages/core/domains/group/split.go. Keep the two in step:
// the defaultSplits cases of spec/vectors.json run against both.

/// Methods a group's default split may use: only those that do not depend
/// on the expense total.
const defaultSplitMethods = {'equal', 'percent', 'shares'};

const _field = 'defaultSplit';

/// Validates a default split against the group's current [members] and
/// returns its values trimmed.
///
/// * `equal`: [values] is empty and everyone splits equally.
/// * `percent` and `shares`: [values] has an entry for every member and for
///   nobody else; plain decimals with at most four decimal places, never
///   negative, `0` leaves that member out. Percentages add up to exactly
///   100, shares to more than zero.
Map<String, String> validateDefaultSplit(
  String method,
  Map<String, String> values,
  Iterable<String> members,
) {
  if (method == 'equal') {
    if (values.isNotEmpty) {
      throw const LedgerError(
        _field,
        'An equal default split takes no values.',
      );
    }
    return const {};
  }
  if (method != 'percent' && method != 'shares') {
    throw const LedgerError(
      _field,
      'A default split is equal, percent or shares.',
    );
  }
  final current = members.toSet();
  final out = <String, String>{};
  var sum = Rational.zero;
  for (final entry in values.entries) {
    if (!current.contains(entry.key)) {
      throw const LedgerError(
        _field,
        'Only current members can be part of the default split.',
      );
    }
    final text = entry.value.trim();
    final r = Rational.tryParseDecimal(text);
    if (r == null || !(r * Rational.integer(10000)).isInteger) {
      throw const LedgerError(
        _field,
        'Use plain decimal numbers with at most four decimal places.',
      );
    }
    if (r.sign < 0) {
      throw const LedgerError(_field, 'Values cannot be negative.');
    }
    sum = sum + r;
    out[entry.key] = text;
  }
  if (out.length != current.length) {
    throw const LedgerError(
      _field,
      'Give every member a value (0 leaves them out).',
    );
  }
  if (method == 'percent' && sum != Rational.integer(100)) {
    throw const LedgerError(_field, 'Percentages must add up to 100.');
  }
  if (sum.sign == 0) {
    throw const LedgerError(_field, 'At least one share must be above zero.');
  }
  return out;
}

/// Whether a saved default split still applies to [members]: always for
/// `equal`; for `percent` and `shares` only while its members are exactly
/// the group's members. A default that no longer matches is treated as
/// absent, so it never silently produces a wrong split.
bool defaultSplitMatches(
  String method,
  Map<String, String> values,
  Iterable<String> members,
) {
  if (method == 'equal') return true;
  final current = members.toSet();
  return values.length == current.length && current.every(values.containsKey);
}
