import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/sync/outbox_sync.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/account/presentation/local_mode_card.dart';
import 'package:splitspends/features/account/presentation/providers.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/features/push/presentation/providers.dart';
import 'package:splitspends/features/push/presentation/push_section.dart';
import 'package:splitspends/features/push/presentation/push_view.dart';

class AccountScreen extends ConsumerWidget {
  const AccountScreen({super.key});

  Future<void> _update(
    BuildContext context,
    WidgetRef ref, {
    String? name,
    String? defaultCurrency,
    String? avatarColor,
    bool? gravatar,
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
              gravatar: gravatar,
              notifications: notifications,
            );
      }
      await ref.read(sessionProvider.notifier).updateMe(me);
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  Future<void> _signOut(BuildContext context, WidgetRef ref) async {
    final waiting = (await ref.read(outboxProvider).pending()).length;
    if (!context.mounted) return;
    if (waiting > 0) {
      final ok = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('Sign out anyway?'),
          content: Text(
            '$waiting change${waiting == 1 ? ' has' : 's have'} not reached '
            'the server yet. Signing out discards '
            '${waiting == 1 ? 'it' : 'them'}.',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Stay signed in'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Sign out'),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }
    // This device should stop getting the account's notifications.
    await ref.read(pushActionsProvider).forget();
    await ref.read(sessionProvider.notifier).signOut();
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
                              local ? 'On this device' : (me.email ?? ''),
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
                if (!local &&
                    me.email != null &&
                    ref.watch(configValueProvider).gravatarEnabled)
                  SwitchListTile(
                    key: const Key('gravatar'),
                    secondary: const Icon(Icons.face_retouching_natural),
                    title: const Text('Use my Gravatar picture'),
                    subtitle: const Text(
                      'Shows the picture gravatar.com has for your email '
                      'instead of your initials. Gravatar can see when '
                      'people load it. Set one up at gravatar.com.',
                    ),
                    isThreeLine: true,
                    value: me.gravatar,
                    onChanged: (v) => _update(context, ref, gravatar: v),
                  ),
                const SectionHeader('Preferences'),
                ListTile(
                  leading: const Icon(Icons.currency_exchange_rounded),
                  title: const Text('Currency'),
                  subtitle: const Text(
                    'Used for expenses outside groups and for new groups. '
                    'Each group has its own currency in its settings.',
                  ),
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
                if (!local && me.canApprove)
                  ListTile(
                    key: const Key('administration'),
                    leading: Icon(
                      me.isAdmin
                          ? Icons.admin_panel_settings_outlined
                          : Icons.how_to_reg_outlined,
                    ),
                    title: Text(me.isAdmin ? 'Administration' : 'Approvals'),
                    subtitle: Text(
                      me.isAdmin
                          ? 'Sign-up, approvals, roles and accounts'
                          : 'Approve people who want to join',
                    ),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => context.push(Routes.admin),
                  ),
                ListTile(
                  key: const Key('manage-categories'),
                  leading: const Icon(Icons.category_outlined),
                  title: const Text('Categories'),
                  subtitle: const Text('Add your own expense categories'),
                  trailing: const Icon(Icons.chevron_right_rounded),
                  onTap: () => context.push(Routes.categories),
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
                const _ThemeColors(),
                if (local) ...[
                  const LocalModeCard(),
                  ListTile(
                    leading: const Icon(Icons.notifications_off_outlined),
                    title: const Text('Push notifications'),
                    subtitle: Text(
                      PushView.resolve(
                        connected: false,
                        serverEnabled: false,
                      ).message,
                    ),
                  ),
                ] else ...[
                  const SectionHeader('Notifications'),
                  for (final kind in NotificationKind.values)
                    _KindTile(
                      kind: kind,
                      prefs: prefs,
                      push: ref.watch(configValueProvider).pushEnabled,
                      onChanged: (next) =>
                          _update(context, ref, notifications: next),
                    ),
                  const PushSection(),
                  const SectionHeader('Developers'),
                  ListTile(
                    leading: const Icon(Icons.key_rounded),
                    title: const Text('API tokens'),
                    subtitle: const Text('Connect AI assistants through MCP'),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => context.push(Routes.tokens),
                  ),
                  const LocalCopyCard(),
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
                    onTap: () => _signOut(context, ref),
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

/// One notification kind with its email and push switches.
class _KindTile extends StatelessWidget {
  const _KindTile({
    required this.kind,
    required this.prefs,
    required this.push,
    required this.onChanged,
  });

  final NotificationKind kind;
  final NotificationPrefs prefs;

  /// Whether the server sends push, so the push switch means something.
  final bool push;
  final ValueChanged<NotificationPrefs> onChanged;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      title: Text(kind.label),
      trailing: Wrap(
        spacing: 8,
        children: [
          FilterChip(
            label: const Text('Email'),
            selected: prefs.email.on(kind),
            onSelected: (v) =>
                onChanged(prefs.copyWith(email: prefs.email.set(kind, v))),
          ),
          if (push)
            FilterChip(
              label: const Text('Push'),
              selected: prefs.push.on(kind),
              onSelected: (v) =>
                  onChanged(prefs.copyWith(push: prefs.push.set(kind, v))),
            ),
        ],
      ),
    );
  }
}

/// Accent colour swatches, saved on this device.
class _ThemeColors extends ConsumerWidget {
  const _ThemeColors();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final current = ref.watch(themeColorProvider);
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(72, 0, 16, 12),
      child: Wrap(
        spacing: 10,
        runSpacing: 10,
        children: [
          for (final c in ThemeColor.values)
            Tooltip(
              message: c.label,
              excludeFromSemantics: true,
              child: Semantics(
                button: true,
                selected: c == current,
                label: '${c.label} theme',
                child: InkWell(
                  key: Key('theme-color-${c.name}'),
                  customBorder: const CircleBorder(),
                  onTap: () => ref.read(themeColorProvider.notifier).set(c),
                  child: Container(
                    width: 32,
                    height: 32,
                    decoration: BoxDecoration(
                      color: c.seed,
                      shape: BoxShape.circle,
                      border: Border.all(
                        color: c == current
                            ? scheme.onSurface
                            : Colors.transparent,
                        width: 2,
                      ),
                    ),
                    child: c == current
                        ? const Icon(Icons.check, size: 16, color: Colors.white)
                        : null,
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
