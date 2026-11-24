import 'package:intl/intl.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_web_plugins/url_strategy.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/features/push/platform/push_platform.dart';

/// Starts the app. [args] come from the platform shell: Android passes
/// `--unifiedpush-bg` when it starts the app only to handle a push event.
Future<void> main([List<String> args = const []]) async {
  if (await runPushBackground(args)) return;
  WidgetsFlutterBinding.ensureInitialized();
  // The app is English-only: dates and numbers never follow the device locale.
  Intl.defaultLocale = 'en_US';
  // Clean URLs on web (/groups/abc); the server falls back to index.html.
  usePathUrlStrategy();
  runApp(
    ProviderScope(
      // Errors surface in the UI with a retry button; no silent retry loops.
      retry: (retryCount, error) => null,
      child: const SplitSpendsApp(),
    ),
  );
}
