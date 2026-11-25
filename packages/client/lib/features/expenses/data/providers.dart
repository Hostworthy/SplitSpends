import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/mode.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/data/offline_expenses_repository.dart';
import 'package:splitspends/features/expenses/data/remote_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expenses_repository.dart';

final expensesRepositoryProvider = Provider<ExpensesRepository>(
  (ref) => byMode(
    ref,
    remote: (api) => OfflineExpensesRepository(
      RemoteExpensesRepository(api),
      ref.watch(outboxProvider),
      meId: () => ref.read(meProvider)?.id ?? '',
    ),
    local: () => LocalExpensesRepository(ref.watch(localLedgerProvider)),
  ),
);
