import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/auth/data/remote_auth_repository.dart';
import 'package:splitspends/features/auth/domain/auth_repository.dart';

/// Login always talks to the server being connected to.
final authRepositoryProvider = Provider<AuthRepository>(
  (ref) => RemoteAuthRepository(ref.read(httpClientProvider)),
);
