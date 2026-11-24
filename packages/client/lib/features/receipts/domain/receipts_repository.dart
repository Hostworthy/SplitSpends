import 'dart:typed_data';

import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';

/// Receipt images and AI helpers (AI needs a server that enables it).
abstract interface class ReceiptsRepository {
  Future<Receipt> upload(Uint8List bytes, String filename, String contentType);

  Future<Uint8List> image(String id);

  /// Asks the server's AI provider to turn instructions, and optionally an
  /// uploaded receipt, into a draft expense. Never saves it.
  Future<ExpenseInput> draft(String text, {String? groupId, String? receiptId});
}
