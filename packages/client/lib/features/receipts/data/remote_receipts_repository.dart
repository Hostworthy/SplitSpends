import 'dart:typed_data';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/receipts/domain/receipts_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';

/// HTTP implementation used in connected mode.
class RemoteReceiptsRepository implements ReceiptsRepository {
  RemoteReceiptsRepository(this._api);

  final ApiClient _api;

  @override
  Future<Receipt> upload(
    Uint8List bytes,
    String filename,
    String contentType,
  ) => _api.upload(
    '/receipts',
    field: 'file',
    bytes: bytes,
    filename: filename,
    contentType: contentType,
    parse: (d) => Receipt.fromJson(d as JsonMap),
  );

  @override
  Future<Uint8List> image(String id) => _api.getBytes('/receipts/$id');

  @override
  Future<ExpenseInput> draft(
    String text, {
    String? groupId,
    String? receiptId,
  }) => _api.postData('/ai/expense-draft', {
    'text': text,
    'groupId': ?groupId,
    'receiptId': ?receiptId,
  }, (d) => ExpenseInput.fromJson(d as JsonMap));
}
