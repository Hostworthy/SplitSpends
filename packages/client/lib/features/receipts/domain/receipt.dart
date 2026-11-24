import 'package:splitspends/core/models/json.dart';

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
