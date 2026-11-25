import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/app/theme_settings.dart';

void main() {
  test(
    'every accent builds light and dark themes with fixed money colours',
    () {
      for (final c in ThemeColor.values) {
        for (final theme in [AppTheme.light(c), AppTheme.dark(c)]) {
          expect(theme.colorScheme.primary, isNot(theme.colorScheme.onPrimary));
          final money = theme.extension<BalanceColors>()!;
          final base =
              (theme.brightness == AppTheme.light().brightness
                      ? AppTheme.light()
                      : AppTheme.dark())
                  .extension<BalanceColors>()!;
          expect(money.owed, base.owed, reason: c.name);
          expect(money.owe, base.owe, reason: c.name);
        }
      }
      expect(
        AppTheme.light(ThemeColor.blue).colorScheme.primary,
        isNot(AppTheme.light().colorScheme.primary),
      );
    },
  );

  test('the chosen accent is remembered on the device', () async {
    final store = MemoryLocalStore(themeColor: 'purple');
    final container = ProviderContainer(
      overrides: [localStoreProvider.overrideWithValue(store)],
    );
    addTearDown(container.dispose);
    expect(container.read(themeColorProvider), ThemeColor.teal);
    await Future<void>.delayed(Duration.zero);
    expect(container.read(themeColorProvider), ThemeColor.purple);
    await container.read(themeColorProvider.notifier).set(ThemeColor.amber);
    expect(store.themeColor, 'amber');
    expect(ThemeColor.named('nonsense'), ThemeColor.teal);
  });
}
