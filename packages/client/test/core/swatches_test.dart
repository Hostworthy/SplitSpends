import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/widgets/swatches.dart';

void main() {
  testWidgets('each swatch is a named button that says whether it is chosen', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    String? picked;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Swatches(
            selected: '#00897b',
            onSelected: (hex) => picked = hex,
          ),
        ),
      ),
    );

    for (final s in swatches) {
      expect(
        tester.getSemantics(find.bySemanticsLabel('${s.name} color')),
        isSemantics(isButton: true, isSelected: s.name == 'Teal'),
        reason: s.name,
      );
    }

    await tester.tap(find.bySemanticsLabel('Blue color'));
    expect(picked, '#1E88E5');
    handle.dispose();
  });
}
