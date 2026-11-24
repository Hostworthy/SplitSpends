import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/account/domain/api_token.dart';

final myBalancesProvider = FutureProvider<BalanceSummary>(
  (ref) => ref.watch(balancesRepositoryProvider).mine(),
);

final tokensProvider = FutureProvider<List<ApiToken>>(
  (ref) => ref.watch(tokensRepositoryProvider).list(),
);

final themeModeProvider = NotifierProvider<ThemeModeController, ThemeMode>(
  ThemeModeController.new,
);

class ThemeModeController extends Notifier<ThemeMode> {
  @override
  ThemeMode build() {
    _load();
    return ThemeMode.system;
  }

  Future<void> _load() async {
    final saved = await ref.read(localStoreProvider).readThemeMode();
    final mode = ThemeMode.values.where((m) => m.name == saved).firstOrNull;
    if (mode != null && ref.mounted) state = mode;
  }

  Future<void> set(ThemeMode mode) async {
    state = mode;
    await ref.read(localStoreProvider).writeThemeMode(mode.name);
  }
}
