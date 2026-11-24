import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/rates/rates.dart';
import 'package:splitspends/core/widgets/pickers.dart';

/// The one currency balances and totals are shown in, and where exchange
/// rates come from. Expenses always keep the currency they were entered in.
class CurrencySettings extends ConsumerWidget {
  const CurrencySettings({super.key, required this.onChanged});

  final Future<void> Function(String code) onChanged;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final currency = ref.watch(displayCurrencyProvider);
    final rates = ref.watch(ratesProvider);
    final local = ref.watch(appModeProvider) == AppMode.local;
    final text = Theme.of(context).textTheme;
    final table = rates.value;

    final String status;
    if (rates.isLoading) {
      status = 'Loading exchange rates…';
    } else if (table == null) {
      status = local
          ? 'Exchange rates could not be downloaded. Amounts are shown in '
                'their own currencies until they can.'
          : 'Exchange rates are off or unavailable on this server, so '
                'amounts are shown in their own currencies.';
    } else {
      final when = table.publishedAt ?? table.fetchedAt;
      final host = Uri.tryParse(table.source)?.host ?? '';
      status =
          'Rates from ${host.isEmpty ? 'the server' : host}'
          '${when == null ? '' : ', updated ${DateFormat.yMMMd().format(when.toLocal())}'}.'
          '${local ? ' Only the rate list is downloaded; nothing about you is sent.' : ''}';
    }

    return Column(
      children: [
        ListTile(
          leading: const Icon(Icons.currency_exchange_rounded),
          title: const Text('Currency'),
          subtitle: const Text(
            'New expenses start in this currency, and balances and totals '
            'are shown in it. Each expense keeps the currency it was '
            'entered in.',
          ),
          isThreeLine: true,
          trailing: Text(currency, style: text.titleMedium),
          onTap: () async {
            final code = await pickCurrency(context, selected: currency);
            if (code != null) await onChanged(code);
          },
        ),
        ListTile(
          dense: true,
          leading: const SizedBox(width: 24),
          title: Text(status, style: text.bodySmall),
          trailing: table == null
              ? IconButton(
                  tooltip: 'Try again',
                  icon: const Icon(Icons.refresh_rounded),
                  onPressed: () => ref.invalidate(ratesProvider),
                )
              : null,
          onTap: table == null || table.source.isEmpty
              ? null
              : () => launchUrl(Uri.parse(table.source)),
        ),
      ],
    );
  }
}
