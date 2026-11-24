import 'package:intl/intl.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_web_plugins/url_strategy.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';

void main() {
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
