import 'dart:typed_data';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';
import 'package:splitspends/features/receipts/domain/receipts_repository.dart';

/// Receipts and AI run on a server; on-device mode explains that calmly.
class LocalReceiptsRepository implements ReceiptsRepository {
  const LocalReceiptsRepository();

  static const _error = ApiError.needsServer(
    'Receipt photos and AI help are features of a SplitSpends server. '
    'Connect to one to use them.',
  );

  @override
  Future<Receipt> upload(
    Uint8List bytes,
    String filename,
    String contentType,
  ) => Future.error(_error);

  @override
  Future<Uint8List> image(String id) => Future.error(_error);

  @override
  Future<ExpenseInput> draft(
    String text, {
    String? groupId,
    String? receiptId,
  }) => Future.error(_error);
}
