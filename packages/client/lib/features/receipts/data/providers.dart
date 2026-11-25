import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/receipts/data/local_receipts_repository.dart';
import 'package:splitspends/features/receipts/data/remote_receipts_repository.dart';
import 'package:splitspends/features/receipts/domain/receipts_repository.dart';

/// Receipts and AI need a server (AI is a server-side feature).
final receiptsRepositoryProvider = Provider<ReceiptsRepository>(
  (ref) => byMode(
    ref,
    remote: RemoteReceiptsRepository.new,
    local: () => const LocalReceiptsRepository(),
  ),
);
