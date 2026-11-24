import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/features/auth/presentation/auth_layout.dart';

/// On-device mode: just a name (to label "you" in splits) and a currency.
class LocalSetupScreen extends ConsumerStatefulWidget {
  const LocalSetupScreen({super.key});

  @override
  ConsumerState<LocalSetupScreen> createState() => _LocalSetupScreenState();
}

class _LocalSetupScreenState extends ConsumerState<LocalSetupScreen> {
  final _name = TextEditingController();
  String _currency = 'USD';
  String? _error;
  bool _busy = false;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _start() async {
    final name = _name.text.trim();
    if (name.isEmpty) {
      setState(() => _error = 'Enter a name so splits can say who is who.');
      return;
    }
    setState(() => _busy = true);
    await ref
        .read(sessionProvider.notifier)
        .useLocally(name: name, currency: _currency);
  }

  @override
  Widget build(BuildContext context) {
    return AuthLayout(
      showBack: true,
      title: 'Use on this device',
      subtitle: 'Nothing leaves this device. You can connect to a server later to sync or share.',
      children: [
        TextField(
          key: const Key('local-name'),
          controller: _name,
          autofocus: true,
          textCapitalization: TextCapitalization.words,
          textInputAction: TextInputAction.done,
          onSubmitted: (_) => _start(),
          decoration: InputDecoration(
            labelText: 'Your name',
            prefixIcon: const Icon(Icons.badge_outlined),
            errorText: _error,
          ),
        ),
        const SizedBox(height: 12),
        ListTile(
          contentPadding: EdgeInsets.zero,
          leading: const Icon(Icons.currency_exchange_rounded),
          title: const Text('Default currency'),
          trailing: Text(
            _currency,
            style: Theme.of(context).textTheme.titleMedium,
          ),
          onTap: () async {
            final code = await pickCurrency(context, selected: _currency);
            if (code != null) setState(() => _currency = code);
          },
        ),
        const SizedBox(height: 24),
        FilledButton(
          key: const Key('local-start'),
          onPressed: _busy ? null : _start,
          child: const Text('Get started'),
        ),
      ],
    );
  }
}
