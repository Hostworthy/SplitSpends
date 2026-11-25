import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/admin/data/remote_admin_repository.dart';
import 'package:splitspends/features/admin/domain/admin.dart';

/// Administration exists only on a server.
final adminRepositoryProvider = Provider<AdminRepository>(
  (ref) => byMode(ref, remote: RemoteAdminRepository.new),
);
