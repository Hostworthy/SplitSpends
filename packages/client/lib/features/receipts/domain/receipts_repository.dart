import 'dart:typed_data';

import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';

/// Receipt images and AI helpers (AI needs a server that enables it).
abstract interface class ReceiptsRepository {
  Future<Receipt> upload(Uint8List bytes, String filename, String contentType);

  Future<Uint8List> image(String id);

  Future<ReceiptScan> scan(String id, {String? currency});

  /// Turns free text into a draft expense; never saves it.
  Future<ExpenseInput> draft(String text, {String? groupId});
}
