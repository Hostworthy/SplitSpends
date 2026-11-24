import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';

class Receipt {
  const Receipt({required this.id, this.contentType, this.size = 0});

  factory Receipt.fromJson(JsonMap json) => Receipt(
    id: json['id'] as String,
    contentType: json['contentType'] as String?,
    size: (json['size'] as num?)?.toInt() ?? 0,
  );

  final String id;
  final String? contentType;
  final int size;
}

/// AI extraction of a receipt photo.
class ReceiptScan {
  const ReceiptScan({
    required this.currency,
    this.merchant,
    this.date,
    this.items = const [],
    this.subtotal = 0,
    this.tax = 0,
    this.tip = 0,
    this.total = 0,
    this.category,
  });

  factory ReceiptScan.fromJson(JsonMap json) => ReceiptScan(
    merchant: json['merchant'] as String?,
    date: json['date'] as String?,
    currency: (json['currency'] as String?) ?? 'USD',
    items: (json['items'] as List? ?? const [])
        .cast<JsonMap>()
        .map(LineItem.fromJson)
        .toList(),
    subtotal: (json['subtotal'] as num?)?.toInt() ?? 0,
    tax: (json['tax'] as num?)?.toInt() ?? 0,
    tip: (json['tip'] as num?)?.toInt() ?? 0,
    total: (json['total'] as num?)?.toInt() ?? 0,
    category: json['category'] as String?,
  );

  final String? merchant;
  final String? date;
  final String currency;
  final List<LineItem> items;
  final int subtotal;
  final int tax;
  final int tip;
  final int total;
  final String? category;
}
