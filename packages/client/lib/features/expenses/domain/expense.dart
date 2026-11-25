import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';

enum SplitMethod {
  equal('Equally', 'equal'),
  exact('Exact', 'exact'),
  percent('Percent', 'percent'),
  shares('Shares', 'shares'),
  adjustment('Adjust', 'adjustment'),
  itemized('Items', 'itemized');

  const SplitMethod(this.label, this.wire);

  final String label;
  final String wire;

  static SplitMethod parse(String? value) => SplitMethod.values.firstWhere(
    (m) => m.wire == value,
    orElse: () => SplitMethod.equal,
  );

  static bool known(String? value) =>
      value == null || SplitMethod.values.any((m) => m.wire == value);
}

enum Recurrence {
  none('Never'),
  weekly('Weekly'),
  biweekly('Every 2 weeks'),
  monthly('Monthly'),
  yearly('Yearly');

  const Recurrence(this.label);

  final String label;

  static Recurrence parse(String? value) => Recurrence.values.firstWhere(
    (r) => r.name == value,
    orElse: () => Recurrence.none,
  );

  static bool known(String? value) =>
      value == null || Recurrence.values.any((r) => r.name == value);
}

class Share {
  const Share({
    required this.userId,
    required this.paid,
    required this.owed,
    this.value,
  });

  factory Share.fromJson(JsonMap json) => Share(
    userId: json['userId'] as String,
    paid: (json['paid'] as num?)?.toInt() ?? 0,
    owed: (json['owed'] as num?)?.toInt() ?? 0,
    value: decimalText(json['value']),
  );

  final String userId;
  final int paid;
  final int owed;

  /// The split input for exact / percent / shares / adjustment, as decimal
  /// text (never a float).
  final String? value;

  int get net => paid - owed;
}

class LineItem {
  const LineItem({
    required this.name,
    required this.amount,
    this.userIds = const [],
    this.quantity,
  });

  factory LineItem.fromJson(JsonMap json) => LineItem(
    name: (json['name'] as String?) ?? '',
    amount: (json['amount'] as num?)?.toInt() ?? 0,
    userIds: (json['userIds'] as List? ?? const []).cast<String>(),
    quantity: json['quantity'] as num?,
  );

  final String name;
  final int amount;
  final List<String> userIds;
  final num? quantity;

  LineItem copyWith({String? name, int? amount, List<String>? userIds}) =>
      LineItem(
        name: name ?? this.name,
        amount: amount ?? this.amount,
        userIds: userIds ?? this.userIds,
        quantity: quantity,
      );

  JsonMap toJson() => {'name': name, 'amount': amount, 'userIds': userIds};
}

class Expense {
  const Expense({
    required this.id,
    required this.description,
    required this.currency,
    required this.total,
    required this.date,
    this.groupId,
    this.kind = 'expense',
    this.category,
    this.notes = '',
    this.splitMethod = SplitMethod.equal,
    this.shares = const [],
    this.items = const [],
    this.tax = 0,
    this.tip = 0,
    this.receiptId,
    this.recurrence = Recurrence.none,
    this.unknownMethod,
    this.unknownRecurrence,
    this.createdBy,
    this.updatedBy,
    this.createdAt,
    this.updatedAt,
    this.deleted = false,
    this.commentCount = 0,
    this.myBalance = 0,
    this.pending = false,
  });

  factory Expense.fromJson(JsonMap json) => Expense(
    id: json['id'] as String,
    groupId: json['groupId'] as String?,
    kind: (json['kind'] as String?) ?? 'expense',
    description: (json['description'] as String?) ?? '',
    category: json['category'] as String?,
    currency: (json['currency'] as String?) ?? 'USD',
    total: (json['total'] as num?)?.toInt() ?? 0,
    date: (json['date'] as String?) ?? '',
    notes: (json['notes'] as String?) ?? '',
    splitMethod: SplitMethod.parse(json['splitMethod'] as String?),
    shares: (json['shares'] as List? ?? const [])
        .cast<JsonMap>()
        .map(Share.fromJson)
        .toList(),
    items: (json['items'] as List? ?? const [])
        .cast<JsonMap>()
        .map(LineItem.fromJson)
        .toList(),
    tax: (json['tax'] as num?)?.toInt() ?? 0,
    tip: (json['tip'] as num?)?.toInt() ?? 0,
    receiptId: json['receiptId'] as String?,
    recurrence: Recurrence.parse(json['recurrence'] as String?),
    unknownMethod: SplitMethod.known(json['splitMethod'] as String?)
        ? null
        : json['splitMethod'] as String,
    unknownRecurrence: Recurrence.known(json['recurrence'] as String?)
        ? null
        : json['recurrence'] as String,
    createdBy: json['createdBy'] as String?,
    updatedBy: json['updatedBy'] as String?,
    createdAt: json['createdAt'] as String?,
    updatedAt: json['updatedAt'] as String?,
    deleted: (json['deleted'] as bool?) ?? false,
    commentCount: (json['commentCount'] as num?)?.toInt() ?? 0,
    myBalance: (json['myBalance'] as num?)?.toInt() ?? 0,
  );

  final String id;
  final String? groupId;
  final String kind;
  final String description;
  final String? category;
  final String currency;
  final int total;
  final String date;
  final String notes;
  final SplitMethod splitMethod;
  final List<Share> shares;
  final List<LineItem> items;
  final int tax;
  final int tip;
  final String? receiptId;
  final Recurrence recurrence;

  /// The wire split method or recurrence when this app does not know it
  /// (a newer server). [splitMethod] and [recurrence] then hold only a
  /// fallback, so saving the expense from here would change it.
  final String? unknownMethod;
  final String? unknownRecurrence;

  final String? createdBy;
  final String? updatedBy;
  final String? createdAt;
  final String? updatedAt;
  final bool deleted;
  final int commentCount;
  final int myBalance;

  /// Whether this app can edit the expense without losing how it is split
  /// or how it repeats.
  bool get editable => unknownMethod == null && unknownRecurrence == null;

  /// Shown from a change still waiting in the outbox, not from the server.
  final bool pending;

  bool get isPayment => kind == 'payment';

  DateTime get day => DateTime.tryParse(date) ?? DateTime.now();

  List<Share> get payers => shares.where((s) => s.paid > 0).toList();

  Share? shareOf(String userId) {
    for (final s in shares) {
      if (s.userId == userId) return s;
    }
    return null;
  }

  Expense copyWith({bool? deleted, bool? pending}) => Expense(
    id: id,
    groupId: groupId,
    kind: kind,
    description: description,
    category: category,
    currency: currency,
    total: total,
    date: date,
    notes: notes,
    splitMethod: splitMethod,
    shares: shares,
    items: items,
    tax: tax,
    tip: tip,
    receiptId: receiptId,
    recurrence: recurrence,
    unknownMethod: unknownMethod,
    unknownRecurrence: unknownRecurrence,
    createdBy: createdBy,
    updatedBy: updatedBy,
    createdAt: createdAt,
    updatedAt: updatedAt,
    deleted: deleted ?? this.deleted,
    commentCount: commentCount,
    myBalance: myBalance,
    pending: pending ?? this.pending,
  );
}

class Payer {
  const Payer({required this.userId, required this.amount});

  final String userId;
  final int amount;

  JsonMap toJson() => {'userId': userId, 'amount': amount};
}

class SplitEntry {
  const SplitEntry({required this.userId, this.value});

  final String userId;

  /// Decimal text; the API accepts it as a JSON string or number.
  final String? value;

  JsonMap toJson() => {'userId': userId, 'value': ?value};
}

/// Decimal text for a JSON number or string (ints stay exact).
String? decimalText(Object? value) {
  if (value == null) return null;
  if (value is int) return '$value';
  final text = value.toString().trim();
  return text.isEmpty ? null : text;
}

/// Body of `POST /expenses` and `PUT /expenses/{id}`.
class ExpenseInput {
  const ExpenseInput({
    this.id,
    this.groupId,
    required this.description,
    required this.currency,
    required this.total,
    required this.date,
    this.category,
    this.notes = '',
    this.splitMethod = SplitMethod.equal,
    this.payers = const [],
    this.splits = const [],
    this.items = const [],
    this.tax = 0,
    this.tip = 0,
    this.receiptId,
    this.recurrence = Recurrence.none,
    this.warnings = const [],
  });

  /// Parses an `ExpenseDraft` from the AI endpoint.
  factory ExpenseInput.fromJson(JsonMap json) => ExpenseInput(
    id: json['id'] as String?,
    groupId: json['groupId'] as String?,
    description: (json['description'] as String?) ?? '',
    currency: (json['currency'] as String?) ?? 'USD',
    total: (json['total'] as num?)?.toInt() ?? 0,
    date: (json['date'] as String?) ?? '',
    category: json['category'] as String?,
    notes: (json['notes'] as String?) ?? '',
    splitMethod: SplitMethod.parse(json['splitMethod'] as String?),
    payers: (json['payers'] as List? ?? const [])
        .cast<JsonMap>()
        .map(
          (p) => Payer(
            userId: p['userId'] as String,
            amount: (p['amount'] as num?)?.toInt() ?? 0,
          ),
        )
        .toList(),
    splits: (json['splits'] as List? ?? const [])
        .cast<JsonMap>()
        .map(
          (s) => SplitEntry(
            userId: s['userId'] as String,
            value: decimalText(s['value']),
          ),
        )
        .toList(),
    items: (json['items'] as List? ?? const [])
        .cast<JsonMap>()
        .map(LineItem.fromJson)
        .toList(),
    tax: (json['tax'] as num?)?.toInt() ?? 0,
    tip: (json['tip'] as num?)?.toInt() ?? 0,
    receiptId: json['receiptId'] as String?,
    recurrence: Recurrence.parse(json['recurrence'] as String?),
    warnings: (json['warnings'] as List? ?? const []).cast<String>(),
  );

  /// Prefills an editor from an existing expense.
  factory ExpenseInput.fromExpense(Expense e) => ExpenseInput(
    groupId: e.groupId,
    description: e.description,
    currency: e.currency,
    total: e.total,
    date: e.date,
    category: e.category,
    notes: e.notes,
    splitMethod: e.splitMethod,
    payers: [
      for (final s in e.shares)
        if (s.paid > 0) Payer(userId: s.userId, amount: s.paid),
    ],
    splits: [
      for (final s in e.shares)
        if (s.owed != 0 ||
            s.value != null ||
            e.splitMethod == SplitMethod.itemized)
          SplitEntry(
            userId: s.userId,
            value: e.splitMethod == SplitMethod.exact
                ? (s.value ?? '${s.owed}')
                : s.value,
          ),
    ],
    items: e.items,
    tax: e.tax,
    tip: e.tip,
    receiptId: e.receiptId,
    recurrence: e.recurrence,
  );

  /// Client-chosen id: makes creates idempotent for the offline outbox.
  final String? id;
  final String? groupId;
  final String description;
  final String currency;
  final int total;
  final String date;
  final String? category;
  final String notes;
  final SplitMethod splitMethod;
  final List<Payer> payers;
  final List<SplitEntry> splits;
  final List<LineItem> items;
  final int tax;
  final int tip;
  final String? receiptId;
  final Recurrence recurrence;
  final List<String> warnings;

  ExpenseInput withId(String value) => ExpenseInput(
    id: value,
    groupId: groupId,
    description: description,
    currency: currency,
    total: total,
    date: date,
    category: category,
    notes: notes,
    splitMethod: splitMethod,
    payers: payers,
    splits: splits,
    items: items,
    tax: tax,
    tip: tip,
    receiptId: receiptId,
    recurrence: recurrence,
  );

  JsonMap toJson() => {
    'id': ?id,
    'groupId': groupId,
    'description': description,
    'currency': currency,
    'total': total,
    'date': date,
    'category': ?category,
    'notes': notes,
    'splitMethod': splitMethod.wire,
    'payers': [for (final p in payers) p.toJson()],
    if (splitMethod != SplitMethod.itemized)
      'splits': [for (final s in splits) s.toJson()],
    if (splitMethod == SplitMethod.itemized) ...{
      'items': [for (final i in items) i.toJson()],
      'tax': tax,
      'tip': tip,
    },
    'receiptId': receiptId,
    'recurrence': recurrence.name,
  };
}

class Comment {
  const Comment({
    required this.id,
    required this.expenseId,
    required this.user,
    required this.body,
    required this.createdAt,
  });

  factory Comment.fromJson(JsonMap json) => Comment(
    id: json['id'] as String,
    expenseId: (json['expenseId'] as String?) ?? '',
    user: User.fromJson(json['user'] as JsonMap),
    body: (json['body'] as String?) ?? '',
    createdAt: (json['createdAt'] as String?) ?? '',
  );

  final String id;
  final String expenseId;
  final User user;
  final String body;
  final String createdAt;
}
