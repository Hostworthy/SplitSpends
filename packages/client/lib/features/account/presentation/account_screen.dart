import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/app/theme.dart';
import 'package:splitspends/app/theme_settings.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/account/presentation/local_mode_card.dart';
import 'package:splitspends/features/account/presentation/providers.dart';
import 'package:splitspends/features/account/presentation/sign_out.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/features/push/presentation/push_section.dart';
import 'package:splitspends/features/push/presentation/push_view.dart';

class AccountScreen extends ConsumerWidget {
  const AccountScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final me = ref.watch(meProvider);
    final session = ref.watch(sessionProvider).value;
    if (me == null) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
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
                _ProfileHeader(me: me, local: local),
                _PreferencesSection(me: me, local: local),
                if (local)
                  const _LocalSection()
                else ...[
                  _NotificationsSection(me: me),
                  const _DevelopersSection(),
                  _SessionSection(server: session?.serverUrl ?? ''),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Saves a profile change and explains a failure.
Future<void> _save(
  BuildContext context,
  WidgetRef ref, {
  String? name,
  String? defaultCurrency,
  String? avatarColor,
  bool? gravatar,
  NotificationPrefs Function(NotificationPrefs prefs)? notifications,
}) async {
  try {
    await ref
        .read(profileEditsProvider)
        .save(
          name: name,
          defaultCurrency: defaultCurrency,
          avatarColor: avatarColor,
          gravatar: gravatar,
          notifications: notifications,
        );
  } catch (error) {
    if (context.mounted) showError(context, error);
  }
}

/// Picture, name and email, the avatar colour and the Gravatar switch.
class _ProfileHeader extends ConsumerWidget {
  const _ProfileHeader({required this.me, required this.local});

  final Me me;
  final bool local;

  Future<void> _editName(BuildContext context, WidgetRef ref) async {
    final name = await showDialog<String>(
      context: context,
      builder: (context) => _NameDialog(name: me.name),
    );
    if (name != null && name.trim().isNotEmpty && context.mounted) {
      await _save(context, ref, name: name.trim());
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final text = Theme.of(context).textTheme;
    return Column(
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
                onPressed: () => _editName(context, ref),
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
                  onTap: () => _save(context, ref, avatarColor: toHexColor(c)),
                  child: CircleAvatar(
                    radius: 15,
                    backgroundColor: c,
                    child: me.avatarColor?.toUpperCase() == toHexColor(c)
                        ? const Icon(Icons.check, size: 16, color: Colors.white)
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
            onChanged: (v) => _save(context, ref, gravatar: v),
          ),
      ],
    );
  }
}

/// Currency, administration, categories and the theme.
class _PreferencesSection extends ConsumerWidget {
  const _PreferencesSection({required this.me, required this.local});

  final Me me;
  final bool local;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final themeMode = ref.watch(themeModeProvider);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Preferences'),
        ListTile(
          leading: const Icon(Icons.currency_exchange_rounded),
          title: const Text('Currency'),
          subtitle: const Text(
            'Used for expenses outside groups and for new groups. '
            'Each group has its own currency in its settings.',
          ),
          trailing: Text(
            me.defaultCurrency,
            style: Theme.of(context).textTheme.titleMedium,
          ),
          onTap: () async {
            final code = await pickCurrency(
              context,
              selected: me.defaultCurrency,
            );
            if (code != null && context.mounted) {
              await _save(context, ref, defaultCurrency: code);
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
      ],
    );
  }
}

/// On-device mode: how to move to a server, and why there is no push.
class _LocalSection extends StatelessWidget {
  const _LocalSection();

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const LocalModeCard(),
        ListTile(
          leading: const Icon(Icons.notifications_off_outlined),
          title: const Text('Push notifications'),
          subtitle: Text(
            PushView.resolve(connected: false, serverEnabled: false).message,
          ),
        ),
      ],
    );
  }
}

/// Email and push switches per kind, and push on this device.
class _NotificationsSection extends ConsumerWidget {
  const _NotificationsSection({required this.me});

  final Me me;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final push = ref.watch(configValueProvider).pushEnabled;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Notifications'),
        for (final kind in me.notificationKinds)
          _KindTile(
            kind: kind,
            prefs: me.notifications,
            push: push,
            onChanged: (change) => _save(context, ref, notifications: change),
          ),
        const PushSection(),
      ],
    );
  }
}

/// API tokens and the local copy of your data.
class _DevelopersSection extends StatelessWidget {
  const _DevelopersSection();

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Developers'),
        ListTile(
          leading: const Icon(Icons.key_rounded),
          title: const Text('API tokens'),
          subtitle: const Text('Connect AI assistants through MCP'),
          trailing: const Icon(Icons.chevron_right_rounded),
          onTap: () => context.push(Routes.tokens),
        ),
        const LocalCopyCard(),
      ],
    );
  }
}

/// The server you are signed in to, and signing out.
class _SessionSection extends ConsumerWidget {
  const _SessionSection({required this.server});

  final String server;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final error = Theme.of(context).colorScheme.error;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Session'),
        ListTile(
          leading: const Icon(Icons.dns_outlined),
          title: const Text('Server'),
          subtitle: Text(server),
        ),
        ListTile(
          leading: Icon(Icons.logout_rounded, color: error),
          title: Text('Sign out', style: TextStyle(color: error)),
          onTap: () => signOut(context, ref),
        ),
      ],
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

  /// Receives the change rather than the new settings, so it applies to
  /// the latest settings, not the ones this tile was built with.
  final ValueChanged<NotificationPrefs Function(NotificationPrefs prefs)>
  onChanged;

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
                onChanged((p) => p.copyWith(email: p.email.set(kind, v))),
          ),
          if (push)
            FilterChip(
              label: const Text('Push'),
              selected: prefs.push.on(kind),
              onSelected: (v) =>
                  onChanged((p) => p.copyWith(push: p.push.set(kind, v))),
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

/// Asks for your name. It owns its controller, so the field outlives the
/// dialog's closing animation.
class _NameDialog extends StatefulWidget {
  const _NameDialog({required this.name});

  final String name;

  @override
  State<_NameDialog> createState() => _NameDialogState();
}

class _NameDialogState extends State<_NameDialog> {
  late final _controller = TextEditingController(text: widget.name);

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Your name'),
      content: TextField(
        controller: _controller,
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
          onPressed: () => Navigator.pop(context, _controller.text),
          child: const Text('Save'),
        ),
      ],
    );
  }
}
