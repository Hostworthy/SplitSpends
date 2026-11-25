import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/core/widgets/amount_field.dart';

Widget _field(String currency, ValueChanged<int?> onChanged) => ProviderScope(
  overrides: [configValueProvider.overrideWithValue(localConfig)],
  child: MaterialApp(
    home: Scaffold(
      body: AmountField(currency: currency, onChanged: onChanged),
    ),
  ),
);

void main() {
  testWidgets('shows how a grouped amount was read', (tester) async {
    int? reported;
    await tester.pumpWidget(_field('USD', (v) => reported = v));
    await tester.enterText(find.byType(TextField), '1,234');
    await tester.pump();
    expect(reported, 123400);
    expect(find.text(r'$1,234.00'), findsOneWidget);

    await tester.enterText(find.byType(TextField), '12');
    await tester.pump();
    expect(find.text(r'$12.00'), findsNothing);
  });
}
