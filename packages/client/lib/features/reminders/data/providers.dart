import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/mode.dart';
import 'package:splitspends/features/reminders/data/remote_reminders_repository.dart';
import 'package:splitspends/features/reminders/domain/reminder.dart';

/// Emailed reminders need a server; on this device people share a text.
final remindersRepositoryProvider = Provider<RemindersRepository>(
  (ref) => byMode(ref, remote: RemoteRemindersRepository.new),
);
