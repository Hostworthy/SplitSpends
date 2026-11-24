import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/account/presentation/providers.dart';

class AccountScreen extends ConsumerWidget {
  const AccountScreen({super.key});

  Future<void> _update(
    BuildContext context,
    WidgetRef ref, {
    String? name,
    String? defaultCurrency,
    String? avatarColor,
    NotificationPrefs? notifications,
  }) async {
    final session = ref.read(sessionProvider).value;
    try {
      final Me me;
      if (session?.isLocal ?? false) {
        me = session!.me!.copyWith(
          name: name,
          defaultCurrency: defaultCurrency,
          avatarColor: avatarColor,
          notifications: notifications,
        );
      } else {
        me = await ref
            .read(accountRepositoryProvider)
            .update(
              name: name,
              defaultCurrency: defaultCurrency,
              avatarColor: avatarColor,
              notifications: notifications,
            );
      }
      await ref.read(sessionProvider.notifier).updateMe(me);
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  Future<void> _editName(BuildContext context, WidgetRef ref, Me me) async {
    final controller = TextEditingController(text: me.name);
    final name = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Your name'),
        content: TextField(
          controller: controller,
          autofocus: true,
          textCapitalization: TextCapitalization.words,
          onSubmitted: (v) => Navigator.pop(context, v),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, controller.text),
            child: const Text('Save'),
          ),
        ],
      ),
    );
    controller.dispose();
    if (name != null && name.trim().isNotEmpty && context.mounted) {
      await _update(context, ref, name: name.trim());
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final me = ref.watch(meProvider);
    final session = ref.watch(sessionProvider).value;
    final themeMode = ref.watch(themeModeProvider);
    final text = Theme.of(context).textTheme;
    if (me == null) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    final prefs = me.notifications;
    final local = session?.isLocal ?? false;
    return Scaffold(
      appBar: AppBar(title: const Text('Account')),
      body: ListView(
        padding: const EdgeInsets.only(bottom: 48),
        children: [
          MaxWidth(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Padding(
                  padding: const EdgeInsets.all(16),
                  child: Row(
                    children: [
                      UserAvatar(user: me, size: 72),
                      const SizedBox(width: 16),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(me.displayName, style: text.titleLarge),
                            Text(
                              local ? 'On this device' : me.email,
                              style: text.bodyMedium,
                            ),
                          ],
                        ),
                      ),
                      IconButton(
                        tooltip: 'Edit name',
                        icon: const Icon(Icons.edit_rounded),
                        onPressed: () => _editName(context, ref, me),
                      ),
                    ],
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: Wrap(
                    spacing: 10,
                    runSpacing: 10,
                    children: [
                      for (final c in palette)
                        InkWell(
                          customBorder: const CircleBorder(),
                          onTap: () =>
                              _update(context, ref, avatarColor: toHexColor(c)),
                          child: CircleAvatar(
                            radius: 15,
                            backgroundColor: c,
                            child:
                                me.avatarColor?.toUpperCase() == toHexColor(c)
                                ? const Icon(
                                    Icons.check,
                                    size: 16,
                                    color: Colors.white,
                                  )
                                : null,
                          ),
                        ),
                    ],
                  ),
                ),
                const SectionHeader('Preferences'),
                ListTile(
                  leading: const Icon(Icons.currency_exchange_rounded),
                  title: const Text('Default currency'),
                  trailing: Text(me.defaultCurrency, style: text.titleMedium),
                  onTap: () async {
                    final code = await pickCurrency(
                      context,
                      selected: me.defaultCurrency,
                    );
                    if (code != null && context.mounted) {
                      await _update(context, ref, defaultCurrency: code);
                    }
                  },
                ),
                ListTile(
                  leading: const Icon(Icons.palette_outlined),
                  title: const Text('Theme'),
                  trailing: SegmentedButton<ThemeMode>(
                    showSelectedIcon: false,
                    segments: const [
                      ButtonSegment(
                        value: ThemeMode.system,
                        icon: Icon(Icons.brightness_auto_rounded),
                      ),
                      ButtonSegment(
                        value: ThemeMode.light,
                        icon: Icon(Icons.light_mode_rounded),
                      ),
                      ButtonSegment(
                        value: ThemeMode.dark,
                        icon: Icon(Icons.dark_mode_rounded),
                      ),
                    ],
                    selected: {themeMode},
                    onSelectionChanged: (v) =>
                        ref.read(themeModeProvider.notifier).set(v.first),
                  ),
                ),
                if (local)
                  const _LocalModeCard()
                else ...[
                  const SectionHeader('Email notifications'),
                  SwitchListTile(
                    title: const Text('Expense added'),
                    value: prefs.expenseAdded,
                    onChanged: (v) => _update(
                      context,
                      ref,
                      notifications: prefs.copyWith(expenseAdded: v),
                    ),
                  ),
                  SwitchListTile(
                    title: const Text('Expense edited or deleted'),
                    value: prefs.expenseUpdated,
                    onChanged: (v) => _update(
                      context,
                      ref,
                      notifications: prefs.copyWith(expenseUpdated: v),
                    ),
                  ),
                  SwitchListTile(
                    title: const Text('Payment received'),
                    value: prefs.paymentReceived,
                    onChanged: (v) => _update(
                      context,
                      ref,
                      notifications: prefs.copyWith(paymentReceived: v),
                    ),
                  ),
                  SwitchListTile(
                    title: const Text('Added to a group'),
                    value: prefs.addedToGroup,
                    onChanged: (v) => _update(
                      context,
                      ref,
                      notifications: prefs.copyWith(addedToGroup: v),
                    ),
                  ),
                  const SectionHeader('Developers'),
                  ListTile(
                    leading: const Icon(Icons.key_rounded),
                    title: const Text('API tokens'),
                    subtitle: const Text('Connect AI assistants through MCP'),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => context.push(Routes.tokens),
                  ),
                  const SectionHeader('Session'),
                  ListTile(
                    leading: const Icon(Icons.dns_outlined),
                    title: const Text('Server'),
                    subtitle: Text(session?.serverUrl ?? ''),
                  ),
                  ListTile(
                    leading: Icon(
                      Icons.logout_rounded,
                      color: Theme.of(context).colorScheme.error,
                    ),
                    title: Text(
                      'Sign out',
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                    onTap: () => ref.read(sessionProvider.notifier).signOut(),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Shown in on-device mode instead of server-only settings.
class _LocalModeCard extends ConsumerWidget {
  const _LocalModeCard();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final scheme = Theme.of(context).colorScheme;
    final text = Theme.of(context).textTheme;
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 20, 16, 0),
      color: scheme.secondaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.smartphone_rounded,
                  color: scheme.onSecondaryContainer,
                ),
                const SizedBox(width: 8),
                Text(
                  'Using SplitSpends on this device',
                  style: text.titleMedium,
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'Your data is stored only here. Connect to a server to sync '
              'between devices, share groups with friends and get email '
              'updates. Your on-device data stays on this device.',
              style: text.bodyMedium,
            ),
            const SizedBox(height: 12),
            FilledButton.tonalIcon(
              onPressed: () => ref.read(sessionProvider.notifier).chooseAgain(),
              icon: const Icon(Icons.cloud_sync_rounded),
              label: const Text('Connect to a server'),
            ),
          ],
        ),
      ),
    );
  }
}
