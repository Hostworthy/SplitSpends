import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/router.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/features/account/presentation/providers.dart';

class SplitSpendsApp extends ConsumerWidget {
  const SplitSpendsApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp.router(
      title: 'SplitSpends',
      locale: const Locale('en', 'US'),
      supportedLocales: const [Locale('en', 'US')],
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light(ref.watch(themeColorProvider)),
      darkTheme: AppTheme.dark(ref.watch(themeColorProvider)),
      themeMode: ref.watch(themeModeProvider),
      routerConfig: ref.watch(routerProvider),
    );
  }
}
