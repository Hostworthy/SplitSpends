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
