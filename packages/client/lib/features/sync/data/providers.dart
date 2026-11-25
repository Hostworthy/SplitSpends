import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/sync/data/backup_service.dart';
import 'package:splitspends/features/sync/data/import_service.dart';

/// Uploads on-device data to an account.
final importServiceProvider = Provider<ImportService>(
  (ref) => ImportService(
    ref.watch(localLedgerProvider),
    ref.read(httpClientProvider),
  ),
);

final backupServiceProvider = Provider<BackupService>(
  (ref) => BackupService(ref.watch(localLedgerProvider)),
);
