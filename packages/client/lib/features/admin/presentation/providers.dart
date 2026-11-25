import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/features/admin/data/providers.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/features/admin/domain/admin.dart';

final signupSettingsProvider = FutureProvider.autoDispose<SignupSettings>(
  (ref) => ref.watch(adminRepositoryProvider).signup(),
);

final signupRequestsProvider = FutureProvider.autoDispose
    .family<List<SignupRequest>, String>(
      (ref, status) =>
          ref.watch(adminRepositoryProvider).requests(status: status),
    );

/// Filters for the account list: status ('' for all) and a search.
typedef AccountFilter = ({String status, String query});

final accountsProvider = FutureProvider.autoDispose
    .family<PageResult<Account>, AccountFilter>(
      (ref, f) => ref
          .watch(adminRepositoryProvider)
          .accounts(status: f.status, query: f.query),
    );
