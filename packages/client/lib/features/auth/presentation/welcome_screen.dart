import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/widgets/logo.dart';
import 'package:splitspends/features/auth/presentation/waiting_changes.dart';

final hostingGuide = Uri.parse(
  'https://github.com/Hostworthy/SplitSpends/blob/main/docs/HOSTING.md',
);

/// First screen: use the app on this device, or connect to a server.
/// Both choices carry the same weight; neither is the "default".
class WelcomeScreen extends ConsumerWidget {
  const WelcomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final wide = MediaQuery.sizeOf(context).width >= 760;

    final local = _ChoiceCard(
      key: const Key('choice-local'),
      icon: Icons.smartphone_rounded,
      title: 'Use on this device',
      points: const [
        'No account needed.',
        'Your data stays on this device.',
        'You can connect to a server later.',
      ],
      action: 'Start',
      onPressed: () => context.push(Routes.welcomeLocal),
    );
    final connect = _ChoiceCard(
      key: const Key('choice-connect'),
      icon: Icons.cloud_sync_rounded,
      title: 'Connect to a server',
      points: const [
        'An account is only needed to sync between devices and share with others.',
        'You need the address of a Split Spends server that you or a friend hosts.',
        'We email you a sign-in code. No password.',
      ],
      action: 'Connect',
      onPressed: () => context.push(Routes.login),
      footer: TextButton.icon(
        onPressed: () => launchUrl(hostingGuide),
        icon: const Icon(Icons.help_outline_rounded, size: 18),
        label: const Text('How do I get a server?'),
      ),
    );

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: BoxConstraints(maxWidth: wide ? 860 : 440),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Align(
                    alignment: Alignment.centerLeft,
                    child: AppLogo(size: 44),
                  ),
                  const SizedBox(height: 28),
                  Text(
                    'Split expenses,\nnot friendships.',
                    style: text.headlineMedium,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Track shared costs with friends, housemates and trips. '
                    'Free for personal use, with every feature included.',
                    style: text.bodyLarge?.copyWith(
                      color: scheme.onSurfaceVariant,
                    ),
                  ),
                  const SizedBox(height: 28),
                  const WaitingChanges(),
                  if (wide)
                    IntrinsicHeight(
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          Expanded(child: local.filled()),
                          const SizedBox(width: 16),
                          Expanded(child: connect.filled()),
                        ],
                      ),
                    )
                  else ...[
                    local,
                    const SizedBox(height: 16),
                    connect,
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _ChoiceCard extends StatelessWidget {
  const _ChoiceCard({
    super.key,
    required this.icon,
    required this.title,
    required this.points,
    required this.action,
    required this.onPressed,
    this.footer,
    this.fill = false,
  });

  /// Same card, pushing its button to the bottom (side-by-side layout).
  _ChoiceCard filled() => _ChoiceCard(
    key: key,
    icon: icon,
    title: title,
    points: points,
    action: action,
    onPressed: onPressed,
    footer: footer,
    fill: true,
  );

  final IconData icon;
  final String title;
  final List<String> points;
  final String action;
  final VoidCallback onPressed;
  final Widget? footer;
  final bool fill;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: scheme.primaryContainer,
                borderRadius: BorderRadius.circular(16),
              ),
              child: Icon(icon, color: scheme.onPrimaryContainer),
            ),
            const SizedBox(height: 16),
            Text(title, style: text.titleLarge),
            const SizedBox(height: 10),
            for (final p in points)
              Padding(
                padding: const EdgeInsets.only(bottom: 6),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Padding(
                      padding: const EdgeInsets.only(top: 2),
                      child: Icon(
                        Icons.check_rounded,
                        size: 18,
                        color: scheme.primary,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(child: Text(p, style: text.bodyMedium)),
                  ],
                ),
              ),
            if (fill) const Spacer() else const SizedBox(height: 12),
            SizedBox(
              width: double.infinity,
              child: FilledButton.tonal(
                onPressed: onPressed,
                child: Text(action),
              ),
            ),
            ?footer,
          ],
        ),
      ),
    );
  }
}
