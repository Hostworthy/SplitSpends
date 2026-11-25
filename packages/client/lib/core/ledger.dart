import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/widgets/states.dart';

/// Counts changes to the ledger (expenses, payments, memberships). Every
/// provider that shows ledger data watches it where it is defined, so one
/// change refetches all of them; only watched providers do any work.
final ledgerVersionProvider = NotifierProvider<LedgerVersion, int>(
  LedgerVersion.new,
);

class LedgerVersion extends Notifier<int> {
  @override
  int build() => 0;

  void bump() => state++;
}

/// Refreshes everything that depends on the ledger after a change.
extension LedgerRefresh on WidgetRef {
  void refreshLedger() => read(ledgerVersionProvider.notifier).bump();
}

/// Same as [LedgerRefresh.refreshLedger] from outside a widget.
void refreshLedgerIn(ProviderContainer container) =>
    container.read(ledgerVersionProvider.notifier).bump();

/// Saves a change to the ledger with [runChange] and refreshes everything
/// that shows it, even when the person left the screen meanwhile.
Future<bool> changeLedger(
  BuildContext context,
  Future<void> Function(ProviderContainer container) change, {
  String? done,
}) => runChange(context, (container) async {
  await change(container);
  refreshLedgerIn(container);
}, done: done);
