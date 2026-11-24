import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/local_config.dart';

/// Server config in connected mode (or while connecting); the bundled
/// catalog on this device otherwise.
final configProvider = FutureProvider<AppConfig>((ref) async {
  final mode = ref.watch(appModeProvider);
  if (mode == AppMode.local) return localConfig;
  final client = ref.watch(apiClientProvider);
  if (client.serverUrl.isEmpty) return localConfig;
  return client.getData('/config', (d) => AppConfig.fromJson(d as JsonMap));
});

/// Config that is always available (bundled defaults while loading).
final configValueProvider = Provider<AppConfig>(
  (ref) => ref.watch(configProvider).value ?? localConfig,
);
