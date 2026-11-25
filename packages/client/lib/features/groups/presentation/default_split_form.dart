import 'package:material_ui/material_ui.dart';
import 'package:splitspends_ledger/ledger.dart';

import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/split_calculator.dart';
import 'package:splitspends/features/groups/domain/group.dart';

/// State of the "Default split" section in group settings. Pure Dart plus
/// ChangeNotifier; the rules come from the ledger (the server's rules).
class DefaultSplitForm extends ChangeNotifier {
  DefaultSplitForm({required List<String> members, DefaultSplit? initial})
    : members = List.unmodifiable(members) {
    load(initial);
  }

  /// Current group members, in display order.
  final List<String> members;

  /// Null means no default split.
  SplitMethod? method;

  /// Per-member text for percent and shares; blank counts as 0.
  final Map<String, String> values = {};

  DefaultSplit? _saved;

  void load(DefaultSplit? split) {
    _saved = split;
    method = split?.method;
    values
      ..clear()
      ..addAll(split?.values ?? const {});
    notifyListeners();
  }

  /// Switches method and prefills sensible values: an even percentage
  /// split, or one share each.
  void setMethod(SplitMethod? value) {
    if (value == method) return;
    method = value;
    values.clear();
    if (value == SplitMethod.percent && members.isNotEmpty) {
      // Hundredths of a hundredth of a percent, so parts add up to 100.
      final parts = even(1000000, members.length);
      for (var i = 0; i < members.length; i++) {
        values[members[i]] = percentText(parts[i]);
      }
    } else if (value == SplitMethod.shares) {
      for (final id in members) {
        values[id] = '1';
      }
    }
    notifyListeners();
  }

  void setValue(String userId, String? value) {
    if (value == null || value.trim().isEmpty) {
      values.remove(userId);
    } else {
      values[userId] = value.trim();
    }
    notifyListeners();
  }

  Map<String, String> get _filled => {
    for (final id in members) id: values[id] ?? '0',
  };

  /// Human readable problem preventing save, or null.
  String? get problem {
    final m = method;
    if (m == null) return null;
    try {
      validateDefaultSplit(
        m.wire,
        m == SplitMethod.equal ? const {} : _filled,
        members,
      );
      return null;
    } on LedgerError catch (error) {
      final text = error.message;
      return text.endsWith('.') ? text.substring(0, text.length - 1) : text;
    }
  }

  /// For percent: "40% left", "10% over" or "100% assigned".
  String? get remaining {
    if (method != SplitMethod.percent) return null;
    var sum = Rational.zero;
    for (final v in _filled.values) {
      sum = sum + (Rational.tryParseDecimal(v) ?? Rational.zero);
    }
    return percentLeft(tenThousandths(Rational.integer(100) - sum));
  }

  /// What to save: null clears the default split.
  DefaultSplit? get split {
    final m = method;
    if (m == null) return null;
    return DefaultSplit(
      method: m,
      values: m == SplitMethod.equal ? const {} : _filled,
    );
  }

  /// Whether the form differs from what is saved.
  bool get changed {
    final next = split;
    final saved = _saved;
    if (next == null || saved == null) return next != saved;
    if (next.method != saved.method) return true;
    for (final id in members) {
      final a = Rational.tryParseDecimal(next.values[id] ?? '0');
      final b = Rational.tryParseDecimal(saved.values[id] ?? '0');
      if (a != b) return true;
    }
    return false;
  }

  bool get canSave => changed && problem == null;
}
