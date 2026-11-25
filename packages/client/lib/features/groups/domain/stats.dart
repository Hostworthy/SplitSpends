import 'package:splitspends/core/models/json.dart';

class CategoryAmount {
  const CategoryAmount(this.category, this.amount);

  final String category;
  final int amount;
}

class MonthAmount {
  const MonthAmount(this.month, this.amount);

  final String month;
  final int amount;
}

class MemberSpend {
  const MemberSpend(this.userId, this.paid, this.owed);

  final String userId;
  final int paid;
  final int owed;
}

/// `GET /stats`.
class Stats {
  const Stats({
    required this.currency,
    this.totalSpent = 0,
    this.byCategory = const [],
    this.byMonth = const [],
    this.byMember = const [],
  });

  factory Stats.fromJson(JsonMap json) => Stats(
    currency: (json['currency'] as String?) ?? 'USD',
    totalSpent: (json['totalSpent'] as num?)?.toInt() ?? 0,
    byCategory: [
      for (final c in (json['byCategory'] as List? ?? const []).cast<JsonMap>())
        CategoryAmount(
          (c['category'] as String?) ?? 'general',
          (c['amount'] as num?)?.toInt() ?? 0,
        ),
    ],
    byMonth: [
      for (final m in (json['byMonth'] as List? ?? const []).cast<JsonMap>())
        MonthAmount(m['month'] as String, (m['amount'] as num?)?.toInt() ?? 0),
    ],
    byMember: [
      for (final m in (json['byMember'] as List? ?? const []).cast<JsonMap>())
        MemberSpend(
          m['userId'] as String,
          (m['paid'] as num?)?.toInt() ?? 0,
          (m['owed'] as num?)?.toInt() ?? 0,
        ),
    ],
  );

  final String currency;
  final int totalSpent;
  final List<CategoryAmount> byCategory;
  final List<MonthAmount> byMonth;
  final List<MemberSpend> byMember;

  bool get isEmpty => totalSpent == 0 && byCategory.isEmpty && byMonth.isEmpty;
}

/// [months] (`YYYY-MM`, oldest first) with a zero for every month missing
/// from [first] to [last], so a chart shows a quiet month as a gap instead
/// of drawing its neighbours side by side. The range widens to any month in
/// [months] outside it.
List<MonthAmount> fillMonths(
  List<MonthAmount> months, {
  String? first,
  String? last,
}) {
  final amounts = {for (final m in months) m.month: m.amount};
  final keys = [...amounts.keys, ?first, ?last]..sort();
  if (keys.isEmpty) return const [];
  DateTime? parse(String key) => DateTime.tryParse('$key-01T00:00:00Z');
  final start = parse(keys.first);
  final end = parse(keys.last);
  if (start == null || end == null) return months;
  return [
    for (var d = start; !d.isAfter(end); d = DateTime.utc(d.year, d.month + 1))
      MonthAmount(_month(d), amounts[_month(d)] ?? 0),
  ];
}

String _month(DateTime d) =>
    '${d.year.toString().padLeft(4, '0')}-${d.month.toString().padLeft(2, '0')}';
