import 'package:splitspends_ledger/allocate.dart';
import 'package:splitspends_ledger/rational.dart';

// Dart port of packages/core/domains/expense/split.go. Keep the two
// in step: spec/vectors.json is run against both.

/// Caps a single expense so sums never approach integer limits.
const maxTotal = 1000000000000;

/// Caps the payers, the splits and the people on one item, so the work one
/// expense costs stays small.
const maxPeople = 200;

/// Caps the items of an itemized expense.
const maxItems = 200;

/// The earliest date an expense can have. Dates also stop a year after
/// today, so a typo cannot start a series that runs for centuries.
final firstDay = DateTime.utc(2000);

const defaultCategory = 'general';
const paymentCategory = 'payment';

const _nilId = '00000000-0000-0000-0000-000000000000';

/// A rule violation on one input field (same field names as the API).
class LedgerError implements Exception {
  const LedgerError(this.field, this.message);

  final String field;
  final String message;

  @override
  String toString() => message;
}

class PayerInput {
  const PayerInput(this.userId, this.amount);

  final String userId;
  final int amount;
}

/// One participant's split input; [value] is a decimal string whose meaning
/// depends on the method (null or empty for equal splits).
class SplitValueInput {
  const SplitValueInput(this.userId, [this.value]);

  final String userId;
  final String? value;
}

class ItemInput {
  const ItemInput(this.name, this.amount, this.userIds);

  final String name;
  final int amount;
  final List<String> userIds;
}

/// Everything needed to create or replace an expense.
class ExpenseRequest {
  const ExpenseRequest({
    required this.description,
    required this.currency,
    required this.total,
    this.date,
    this.category = '',
    this.notes = '',
    this.method = 'equal',
    this.payers = const [],
    this.splits = const [],
    this.items = const [],
    this.tax = 0,
    this.tip = 0,
    this.recurrence = 'none',
  });

  final String description;
  final String currency;
  final int total;

  /// `YYYY-MM-DD`; today when null or empty.
  final String? date;
  final String category;
  final String notes;
  final String method;
  final List<PayerInput> payers;
  final List<SplitValueInput> splits;
  final List<ItemInput> items;
  final int tax;
  final int tip;
  final String recurrence;
}

class ShareResult {
  const ShareResult(this.userId, this.paid, this.owed, [this.value]);

  final String userId;
  final int paid;
  final int owed;

  /// The decimal input for exact / percent / shares / adjustment.
  final String? value;
}

/// A validated request with computed shares (sorted by user id).
class NormalizedExpense {
  const NormalizedExpense({
    required this.description,
    required this.currency,
    required this.total,
    required this.date,
    required this.category,
    required this.notes,
    required this.method,
    required this.recurrence,
    required this.items,
    required this.tax,
    required this.tip,
    required this.shares,
  });

  final String description;
  final String currency;
  final int total;
  final DateTime date;
  final String category;
  final String notes;
  final String method;
  final String recurrence;
  final List<ItemInput> items;
  final int tax;
  final int tip;
  final List<ShareResult> shares;
}

const recurrences = {'none', 'weekly', 'biweekly', 'monthly', 'yearly'};

/// Validates [input] and computes every participant's owed amount.
///
/// [currencies] and [categories] are the accepted codes; null accepts any
/// (the vectors do not depend on catalogs).
NormalizedExpense normalize(
  ExpenseRequest input, {
  required DateTime today,
  Set<String>? currencies,
  Set<String>? categories,
}) {
  final description = _collapse(input.description);
  if (description.isEmpty) {
    throw const LedgerError('description', 'Add a description.');
  }
  if (description.runes.length > 120) {
    throw const LedgerError(
      'description',
      'Keep the description under 120 characters.',
    );
  }
  if (input.notes.runes.length > 2000) {
    throw const LedgerError('notes', 'Notes are limited to 2000 characters.');
  }
  final currency = input.currency.trim().toUpperCase();
  if (currency.isEmpty ||
      (currencies != null && !currencies.contains(currency))) {
    throw const LedgerError('currency', 'Unsupported currency.');
  }
  if (input.total <= 0) {
    throw const LedgerError('total', 'The amount must be greater than zero.');
  }
  if (input.total > maxTotal) {
    throw const LedgerError('total', 'The amount is too large.');
  }
  final category = input.category.isEmpty ? defaultCategory : input.category;
  if (categories != null && !categories.contains(category)) {
    throw const LedgerError('category', 'Unknown category.');
  }
  final recurrence = input.recurrence.isEmpty ? 'none' : input.recurrence;
  if (!recurrences.contains(recurrence)) {
    throw const LedgerError('recurrence', 'Unknown recurrence.');
  }
  var date = today;
  if (input.date != null && input.date!.isNotEmpty) {
    date =
        parseDay(input.date!) ??
        (throw const LedgerError('date', 'Use a YYYY-MM-DD date.'));
  }
  if (date.isBefore(firstDay) ||
      date.isAfter(DateTime.utc(today.year + 1, today.month, today.day))) {
    throw const LedgerError(
      'date',
      'Choose a date from 2000 up to a year from today.',
    );
  }

  final paid = _payers(input.payers, input.total);
  final (owed, values, items) = _split(input);
  final itemized = input.method == 'itemized';
  return NormalizedExpense(
    description: description,
    currency: currency,
    total: input.total,
    date: date,
    category: category,
    notes: input.notes,
    method: input.method.isEmpty ? 'equal' : input.method,
    recurrence: recurrence,
    items: itemized ? items : const [],
    tax: itemized ? input.tax : 0,
    tip: itemized ? input.tip : 0,
    shares: _merge(paid, owed, values),
  );
}

Map<String, int> _payers(List<PayerInput> list, int total) {
  if (list.isEmpty) throw const LedgerError('payers', 'Choose who paid.');
  if (list.length > maxPeople) {
    throw const LedgerError(
      'payers',
      'At most $maxPeople people can pay for one expense.',
    );
  }
  final out = <String, int>{};
  var sum = 0;
  for (final p in list) {
    if (_isNil(p.userId)) {
      throw const LedgerError('payers', 'Every payer needs a user.');
    }
    if (out.containsKey(p.userId)) {
      throw const LedgerError('payers', 'A person is listed as payer twice.');
    }
    if (p.amount < 0 || p.amount > maxTotal) {
      throw const LedgerError('payers', 'Paid amounts must be positive.');
    }
    out[p.userId] = p.amount;
    sum += p.amount;
  }
  if (sum != total) {
    throw const LedgerError('payers', 'Paid amounts must add up to the total.');
  }
  return out;
}

(Map<String, int>, Map<String, String>, List<ItemInput>) _split(
  ExpenseRequest input,
) {
  if (input.method == 'itemized') return _itemized(input);
  if (input.splits.isEmpty) {
    throw const LedgerError(
      'splits',
      'Choose who this expense is split between.',
    );
  }
  if (input.splits.length > maxPeople) {
    throw const LedgerError(
      'splits',
      'An expense can be split between at most $maxPeople people.',
    );
  }
  final ids = <String>[];
  // The trimmed values: what is stored is what was parsed.
  final raw = <String>[];
  final seen = <String>{};
  for (final s in input.splits) {
    if (_isNil(s.userId) || !seen.add(s.userId)) {
      throw const LedgerError(
        'splits',
        'Each person can appear in the split once.',
      );
    }
    ids.add(s.userId);
    raw.add(trimSpace(s.value ?? ''));
  }

  final values = <String, String>{};
  late List<int> parts;
  switch (input.method) {
    case 'equal':
    case '':
      parts = even(input.total, ids.length);
    case 'exact':
      parts = [];
      var sum = 0;
      for (var i = 0; i < ids.length; i++) {
        final v = _integer(raw[i], 'splits');
        if (v < 0) {
          throw const LedgerError('splits', 'Amounts cannot be negative.');
        }
        parts.add(v);
        sum += v;
        values[ids[i]] = raw[i];
      }
      if (sum != input.total) {
        throw const LedgerError('splits', 'Amounts must add up to the total.');
      }
    case 'percent':
    case 'shares':
      final weights = <Rational>[];
      var sum = Rational.zero;
      for (var i = 0; i < ids.length; i++) {
        final w = splitDecimal(raw[i]);
        if (w == null || w.sign < 0) {
          throw const LedgerError('splits', 'Use positive numbers.');
        }
        weights.add(w);
        sum = sum + w;
        values[ids[i]] = raw[i];
      }
      if (input.method == 'percent' && sum != Rational.integer(100)) {
        throw const LedgerError('splits', 'Percentages must add up to 100.');
      }
      if (sum.sign == 0) {
        throw const LedgerError(
          'splits',
          'At least one share must be above zero.',
        );
      }
      parts = allocate(input.total, weights);
    case 'adjustment':
      final adjust = <int>[];
      var sum = 0;
      for (var i = 0; i < ids.length; i++) {
        final v = raw[i].isEmpty ? 0 : _integer(raw[i], 'splits');
        adjust.add(v);
        sum += v;
        values[ids[i]] = raw[i];
      }
      final spread = even(input.total - sum, ids.length);
      parts = [];
      for (var i = 0; i < ids.length; i++) {
        final p = adjust[i] + spread[i];
        if (p < 0) {
          throw const LedgerError(
            'splits',
            'Adjustments leave someone owing a negative amount.',
          );
        }
        parts.add(p);
      }
    default:
      throw const LedgerError('splitMethod', 'Unknown split method.');
  }

  return (
    {for (var i = 0; i < ids.length; i++) ids[i]: parts[i]},
    values,
    const <ItemInput>[],
  );
}

/// Each item is shared evenly among its users; tax and tip are spread in
/// proportion to each user's item subtotal.
(Map<String, int>, Map<String, String>, List<ItemInput>) _itemized(
  ExpenseRequest input,
) {
  if (input.items.isEmpty) {
    throw const LedgerError('items', 'Add at least one item.');
  }
  if (input.items.length > maxItems) {
    throw const LedgerError(
      'items',
      'An expense can have at most $maxItems items.',
    );
  }
  if (input.tax < 0 || input.tip < 0) {
    throw const LedgerError('tax', 'Tax and tip cannot be negative.');
  }
  // Bounding each term keeps items plus tax and tip far from overflow.
  if (input.tax > maxTotal || input.tip > maxTotal) {
    throw const LedgerError('tax', 'Tax and tip are too large.');
  }
  final subtotals = <String, int>{};
  final order = <String>[];
  final items = <ItemInput>[];
  var itemsSum = 0;
  for (final raw in input.items) {
    var name = _collapse(raw.name);
    if (name.isEmpty) name = 'Item';
    if (name.runes.length > 120) {
      throw const LedgerError(
        'items',
        'Item names are limited to 120 characters.',
      );
    }
    if (raw.amount <= 0 || raw.amount > maxTotal) {
      throw const LedgerError(
        'items',
        'Item amounts must be greater than zero.',
      );
    }
    if (raw.userIds.isEmpty) {
      throw const LedgerError(
        'items',
        'Assign every item to at least one person.',
      );
    }
    if (raw.userIds.length > maxPeople) {
      throw const LedgerError(
        'items',
        'An item can be shared by at most $maxPeople people.',
      );
    }
    final seen = <String>{};
    for (final id in raw.userIds) {
      if (_isNil(id) || !seen.add(id)) {
        throw const LedgerError(
          'items',
          'Each person can be assigned to an item once.',
        );
      }
    }
    final parts = even(raw.amount, raw.userIds.length);
    for (var j = 0; j < raw.userIds.length; j++) {
      final id = raw.userIds[j];
      if (!subtotals.containsKey(id)) order.add(id);
      subtotals[id] = (subtotals[id] ?? 0) + parts[j];
    }
    itemsSum += raw.amount;
    items.add(ItemInput(name, raw.amount, raw.userIds));
  }
  if (itemsSum + input.tax + input.tip != input.total) {
    throw const LedgerError(
      'total',
      'Items plus tax and tip must equal the total.',
    );
  }
  late List<int> extra;
  try {
    extra = allocate(input.tax + input.tip, [
      for (final id in order) Rational.integer(subtotals[id]!),
    ]);
  } on NoWeightsError {
    throw const LedgerError('items', 'Items must have a positive total.');
  }
  return (
    {
      for (var i = 0; i < order.length; i++)
        order[i]: subtotals[order[i]]! + extra[i],
    },
    const <String, String>{},
    items,
  );
}

List<ShareResult> _merge(
  Map<String, int> paid,
  Map<String, int> owed,
  Map<String, String> values,
) {
  final ids = {...paid.keys, ...owed.keys}.toList()..sort();
  return [
    for (final id in ids)
      ShareResult(
        id,
        paid[id] ?? 0,
        owed[id] ?? 0,
        (values[id] ?? '').isEmpty ? null : values[id],
      ),
  ];
}

/// The longest split value accepted: room for a sign, the 13 digits of
/// [maxTotal], a point and four decimals. Mirrors Go's money.DecimalLength.
const decimalLength = 20;

/// Parses a split value like Go's money.Decimal: a plain decimal such as
/// `60`, `-3.5`, `1.` or `.25` with at most [decimalLength] characters and
/// four decimal places. Returns null for anything else. Trim the input with
/// [trimSpace] first.
Rational? splitDecimal(String input) {
  // Rational.tryParseDecimal trims more than Go does (U+FEFF), so anything
  // left around the value after trimSpace is refused here.
  if (input.length > decimalLength || input.trim() != input) return null;
  final r = Rational.tryParseDecimal(input);
  if (r == null || !(r * Rational.integer(10000)).isInteger) return null;
  return r;
}

int _integer(String input, String field) {
  final r = splitDecimal(input);
  if (r == null || !r.isInteger || r.numerator.bitLength > 63) {
    throw LedgerError(field, 'Amounts must be whole numbers of minor units.');
  }
  final v = r.numerator.toInt();
  if (v > maxTotal || v < -maxTotal) {
    throw LedgerError(field, 'Amount is too large.');
  }
  return v;
}

/// The characters Go's unicode.IsSpace accepts. Dart's String.trim and
/// RegExp `\s` use a different set, so the rules name it explicitly.
const _space =
    '\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000';

final _edges = RegExp('^[$_space]+|[$_space]+\$');

/// Trims like Go's strings.TrimSpace.
String trimSpace(String s) => s.replaceAll(_edges, '');

String _collapse(String s) =>
    s.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).join(' ');

bool _isNil(String id) => id.isEmpty || id == _nilId;

/// Parses `YYYY-MM-DD` strictly into a UTC date.
DateTime? parseDay(String s) {
  final m = RegExp(r'^(\d{4})-(\d{2})-(\d{2})$').firstMatch(s);
  if (m == null) return null;
  final y = int.parse(m[1]!), mo = int.parse(m[2]!), d = int.parse(m[3]!);
  final date = DateTime.utc(y, mo, d);
  if (date.year != y || date.month != mo || date.day != d) return null;
  return date;
}

/// `YYYY-MM-DD` for a date.
String formatDay(DateTime d) =>
    '${d.year.toString().padLeft(4, '0')}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';
