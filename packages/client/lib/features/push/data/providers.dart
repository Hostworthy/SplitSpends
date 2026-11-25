import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/push/data/remote_push_repository.dart';
import 'package:splitspends/features/push/domain/push_repository.dart';

/// Push devices exist only on a server.
final pushRepositoryProvider = Provider<PushRepository>(
  (ref) => byMode(ref, remote: RemotePushRepository.new),
);
