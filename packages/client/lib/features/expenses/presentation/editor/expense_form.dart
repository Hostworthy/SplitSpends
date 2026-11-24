import 'package:material_ui/material_ui.dart';

import 'package:splitspends_ledger/rational.dart';

import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/domain/split_calculator.dart';

/// An editable line item with a stable local key.
class ItemDraft {
  ItemDraft({this.name = '', this.amount = 0, Set<String>? userIds})
    : userIds = userIds ?? {},
      key = UniqueKey();

  final Key key;
  String name;
  int amount;
  final Set<String> userIds;

  LineItem toItem(List<String> order) => LineItem(
    name: name.trim().isEmpty ? 'Item' : name.trim(),
    amount: amount,
    userIds: [
      for (final id in order)
        if (userIds.contains(id)) id,
    ],
  );
}

/// State of the add/edit expense form. Pure Dart + ChangeNotifier so the
/// screen stays a thin view and the rules are easy to test.
class ExpenseForm extends ChangeNotifier {
  ExpenseForm({
    required this.meId,
    required this.currency,
    this.groupId,
    List<String> people = const [],
  }) {
    _people = [...people];
    if (!_people.contains(meId)) _people.insert(0, meId);
    included.addAll(_people);
    payers[meId] = 0;
  }

  final String meId;
  String? groupId;

  /// Everyone who can take part (group members, or you + chosen friends).
  List<String> _people = [];
  List<String> get people => List.unmodifiable(_people);

  String description = '';
  String? category;
  String notes = '';
  int? total;
  String currency;
  DateTime date = Dates.today();
  Recurrence recurrence = Recurrence.none;
  String? receiptId;

  SplitMethod method = SplitMethod.equal;

  /// Who takes part in an `equal` split.
  final Set<String> included = {};

  /// Per-user inputs for exact / percent / shares / adjustment.
  final Map<String, String> values = {};

  /// Payer amounts; with a single payer the amount follows the total.
  final Map<String, int> payers = {};
  bool multiplePayers = false;

  final List<ItemDraft> items = [];
  int tax = 0;
  int tip = 0;

  List<String> warnings = const [];

  // ---------------------------------------------------------------- setters

  void update(VoidCallback change) {
    change();
    notifyListeners();
  }

  /// Replaces the people list (group switch or friends picked).
  void setPeople(List<String> ids, {String? group}) {
    groupId = group;
    final next = [...ids];
    if (!next.contains(meId)) next.insert(0, meId);
    _people = next;
    included
      ..removeWhere((id) => !next.contains(id))
      ..addAll(next.where((id) => !included.contains(id)));
    values.removeWhere((id, _) => !next.contains(id));
    payers.removeWhere((id, _) => !next.contains(id));
    if (payers.isEmpty) payers[meId] = 0;
    for (final item in items) {
      item.userIds.removeWhere((id) => !next.contains(id));
    }
    notifyListeners();
  }

  void setSinglePayer(String id) {
    multiplePayers = false;
    payers
      ..clear()
      ..[id] = total ?? 0;
    notifyListeners();
  }

  void setMultiplePayers(Map<String, int> amounts) {
    multiplePayers = true;
    payers
      ..clear()
      ..addAll(amounts);
    notifyListeners();
  }

  void setMethod(SplitMethod value) {
    if (method == value) return;
    method = value;
    values.clear();
    if (value == SplitMethod.shares) {
      for (final id in _people) {
        values[id] = '1';
      }
    }
    if (value == SplitMethod.itemized && items.isEmpty) {
      items.add(ItemDraft(userIds: {..._people}));
    }
    notifyListeners();
  }

  // -------------------------------------------------------------- computed

  String get singlePayer => payers.keys.firstOrNull ?? meId;

  List<Payer> get payerList {
    if (!multiplePayers) {
      return [Payer(userId: singlePayer, amount: total ?? 0)];
    }
    return [
      for (final e in payers.entries)
        if (e.value != 0) Payer(userId: e.key, amount: e.value),
    ];
  }

  List<String> get splitPeople => method == SplitMethod.equal
      ? _people.where(included.contains).toList()
      : _people;

  int get itemsTotal => items.fold(0, (a, i) => a + i.amount);

  SplitPreview get preview => previewSplit(
    method: method,
    total: total ?? 0,
    participants: splitPeople,
    values: values,
    items: [for (final i in items) i.toItem(_people)],
    tax: tax,
    tip: tip,
  );

  String? get payerError => validatePayers(total ?? 0, payerList);

  /// First problem preventing save, or null.
  String? get problem {
    if (description.trim().isEmpty) return 'Add a description';
    if (total == null || total! <= 0) return 'Enter an amount';
    if (_people.length < 2) return 'Choose who you are splitting with';
    return payerError ?? preview.error;
  }

  bool get canSave => problem == null;

  /// Makes the total match the items when itemizing.
  void syncTotalWithItems() {
    total = itemsTotal + tax + tip;
    if (!multiplePayers) payers[singlePayer] = total!;
    notifyListeners();
  }

  // ------------------------------------------------------------ conversion

  void load(ExpenseInput input) {
    description = input.description;
    category = input.category;
    notes = input.notes;
    total = input.total == 0 ? null : input.total;
    currency = input.currency;
    date = DateTime.tryParse(input.date) ?? Dates.today();
    recurrence = input.recurrence;
    receiptId = input.receiptId;
    method = input.splitMethod;
    warnings = input.warnings;

    final involved = <String>{
      ..._people,
      ...input.payers.map((p) => p.userId),
      ...input.splits.map((s) => s.userId),
      for (final i in input.items) ...i.userIds,
    };
    _people = [..._people, ...involved.where((id) => !_people.contains(id))];

    included
      ..clear()
      ..addAll(
        method == SplitMethod.equal && input.splits.isNotEmpty
            ? input.splits.map((s) => s.userId)
            : _people,
      );
    values.clear();
    for (final s in input.splits) {
      if (s.value != null) values[s.userId] = s.value!;
    }
    payers.clear();
    for (final p in input.payers) {
      payers[p.userId] = p.amount;
    }
    if (payers.isEmpty) payers[meId] = total ?? 0;
    multiplePayers = payers.length > 1;
    items
      ..clear()
      ..addAll(
        input.items.map(
          (i) => ItemDraft(
            name: i.name,
            amount: i.amount,
            userIds: i.userIds.isEmpty ? {..._people} : {...i.userIds},
          ),
        ),
      );
    tax = input.tax;
    tip = input.tip;
    notifyListeners();
  }

  static bool _nonZero(String? value) {
    final r = value == null ? null : Rational.tryParseDecimal(value);
    return r != null && r.sign != 0;
  }

  ExpenseInput toInput() {
    final splits = switch (method) {
      SplitMethod.equal => [
        for (final id in splitPeople) SplitEntry(userId: id),
      ],
      SplitMethod.itemized => const <SplitEntry>[],
      _ => [
        for (final id in _people)
          if (_nonZero(values[id]) || method == SplitMethod.adjustment)
            SplitEntry(userId: id, value: values[id] ?? '0'),
      ],
    };
    return ExpenseInput(
      groupId: groupId,
      description: description.trim(),
      currency: currency,
      total: total ?? 0,
      date: Dates.toApi(date),
      category: category,
      notes: notes.trim(),
      splitMethod: method,
      payers: payerList,
      splits: splits,
      items: [for (final i in items) i.toItem(_people)],
      tax: tax,
      tip: tip,
      receiptId: receiptId,
      recurrence: recurrence,
    );
  }
}
