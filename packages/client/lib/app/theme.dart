import 'package:cupertino_ui/cupertino_ui.dart'
    show CupertinoPageTransitionsBuilder;
import 'package:material_ui/material_ui.dart';

/// Brand seed: a calm emerald teal.
const _seed = Color(0xFF0E7C66);

/// Semantic colors for balances.
@immutable
class BalanceColors extends ThemeExtension<BalanceColors> {
  const BalanceColors({
    required this.owed,
    required this.owe,
    required this.settled,
    required this.owedContainer,
    required this.oweContainer,
  });

  final Color owed;
  final Color owe;
  final Color settled;
  final Color owedContainer;
  final Color oweContainer;

  static BalanceColors of(BuildContext context) =>
      Theme.of(context).extension<BalanceColors>()!;

  Color forAmount(int amount) => amount > 0
      ? owed
      : amount < 0
      ? owe
      : settled;

  @override
  BalanceColors copyWith({
    Color? owed,
    Color? owe,
    Color? settled,
    Color? owedContainer,
    Color? oweContainer,
  }) => BalanceColors(
    owed: owed ?? this.owed,
    owe: owe ?? this.owe,
    settled: settled ?? this.settled,
    owedContainer: owedContainer ?? this.owedContainer,
    oweContainer: oweContainer ?? this.oweContainer,
  );

  @override
  BalanceColors lerp(BalanceColors? other, double t) {
    if (other == null) return this;
    return BalanceColors(
      owed: Color.lerp(owed, other.owed, t)!,
      owe: Color.lerp(owe, other.owe, t)!,
      settled: Color.lerp(settled, other.settled, t)!,
      owedContainer: Color.lerp(owedContainer, other.owedContainer, t)!,
      oweContainer: Color.lerp(oweContainer, other.oweContainer, t)!,
    );
  }
}

abstract final class AppTheme {
  static const radius = 20.0;

  static ThemeData light() => _build(
    ColorScheme.fromSeed(
      seedColor: _seed,
      dynamicSchemeVariant: DynamicSchemeVariant.tonalSpot,
    ).copyWith(surface: const Color(0xFFF5FAF7)),
    const BalanceColors(
      owed: Color(0xFF0B8A5F),
      owe: Color(0xFFD9541E),
      settled: Color(0xFF7B8A85),
      owedContainer: Color(0xFFD7F5E7),
      oweContainer: Color(0xFFFFE3D6),
    ),
  );

  static ThemeData dark() => _build(
    ColorScheme.fromSeed(
      seedColor: _seed,
      brightness: Brightness.dark,
      dynamicSchemeVariant: DynamicSchemeVariant.tonalSpot,
    ).copyWith(surface: const Color(0xFF0F1513)),
    const BalanceColors(
      owed: Color(0xFF5FD9A6),
      owe: Color(0xFFFF9466),
      settled: Color(0xFF8E9C97),
      owedContainer: Color(0xFF0F3A2B),
      oweContainer: Color(0xFF452414),
    ),
  );

  static ThemeData _build(ColorScheme scheme, BalanceColors balances) {
    final base = ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      // Bundled in packages/client, so the family carries the package prefix.
      fontFamily: 'packages/splitspends/Inter',
      visualDensity: VisualDensity.standard,
    );
    final text = base.textTheme.copyWith(
      headlineMedium: base.textTheme.headlineMedium?.copyWith(
        fontWeight: FontWeight.w700,
        letterSpacing: -0.5,
      ),
      headlineSmall: base.textTheme.headlineSmall?.copyWith(
        fontWeight: FontWeight.w700,
        letterSpacing: -0.3,
      ),
      titleLarge: base.textTheme.titleLarge?.copyWith(
        fontWeight: FontWeight.w700,
      ),
      titleMedium: base.textTheme.titleMedium?.copyWith(
        fontWeight: FontWeight.w600,
      ),
    );
    final rounded = RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(radius),
    );
    return base.copyWith(
      textTheme: text,
      scaffoldBackgroundColor: scheme.surface,
      extensions: [balances],
      appBarTheme: AppBarTheme(
        backgroundColor: scheme.surface,
        surfaceTintColor: Colors.transparent,
        scrolledUnderElevation: 0,
        centerTitle: false,
        titleTextStyle: text.titleLarge?.copyWith(color: scheme.onSurface),
      ),
      cardTheme: CardThemeData(
        elevation: 0,
        margin: EdgeInsets.zero,
        color: scheme.surfaceContainerLow,
        shape: rounded,
        clipBehavior: Clip.antiAlias,
      ),
      listTileTheme: ListTileThemeData(
        contentPadding: const EdgeInsets.symmetric(horizontal: 16),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: scheme.surfaceContainerHighest.withValues(alpha: 0.5),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(16),
          borderSide: BorderSide.none,
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(16),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(16),
          borderSide: BorderSide(color: scheme.primary, width: 2),
        ),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 14,
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          minimumSize: const Size(64, 52),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(16),
          ),
          textStyle: text.titleMedium,
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(64, 48),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(16),
          ),
        ),
      ),
      floatingActionButtonTheme: FloatingActionButtonThemeData(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(18)),
      ),
      chipTheme: base.chipTheme.copyWith(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: scheme.surfaceContainer,
        indicatorColor: scheme.primaryContainer,
        height: 68,
        labelBehavior: NavigationDestinationLabelBehavior.alwaysShow,
      ),
      navigationRailTheme: NavigationRailThemeData(
        backgroundColor: scheme.surfaceContainerLow,
        indicatorColor: scheme.primaryContainer,
      ),
      bottomSheetTheme: const BottomSheetThemeData(
        showDragHandle: true,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(28)),
        ),
      ),
      dialogTheme: DialogThemeData(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(28)),
      ),
      snackBarTheme: SnackBarThemeData(
        behavior: SnackBarBehavior.floating,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
      ),
      dividerTheme: DividerThemeData(
        color: scheme.outlineVariant.withValues(alpha: 0.5),
        space: 1,
      ),
      pageTransitionsTheme: const PageTransitionsTheme(
        builders: {
          TargetPlatform.android: FadeForwardsPageTransitionsBuilder(),
          TargetPlatform.iOS: CupertinoPageTransitionsBuilder(),
          TargetPlatform.macOS: CupertinoPageTransitionsBuilder(),
          TargetPlatform.linux: FadeForwardsPageTransitionsBuilder(),
          TargetPlatform.windows: FadeForwardsPageTransitionsBuilder(),
        },
      ),
    );
  }
}

/// Parses `#RRGGBB` colors from the API.
Color? parseHexColor(String? hex) {
  if (hex == null) return null;
  var value = hex.replaceFirst('#', '');
  if (value.length == 6) value = 'FF$value';
  final parsed = int.tryParse(value, radix: 16);
  return parsed == null ? null : Color(parsed);
}

String toHexColor(Color color) {
  final argb = color.toARGB32();
  return '#${(argb & 0xFFFFFF).toRadixString(16).padLeft(6, '0').toUpperCase()}';
}

/// Palette offered for avatars and groups.
const palette = <Color>[
  Color(0xFF0E7C66),
  Color(0xFF00897B),
  Color(0xFF1E88E5),
  Color(0xFF5E35B1),
  Color(0xFF8E24AA),
  Color(0xFFD81B60),
  Color(0xFFE53935),
  Color(0xFFF4511E),
  Color(0xFFFB8C00),
  Color(0xFFC0A000),
  Color(0xFF43A047),
  Color(0xFF546E7A),
];
