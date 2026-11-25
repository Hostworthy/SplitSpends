import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:qr_flutter/qr_flutter.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/utils/share.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/invites/domain/invite.dart';
import 'package:splitspends/features/invites/presentation/providers.dart';

/// "Invite with a link" in group settings: create a link with a QR code,
/// copy or share it, and revoke active links.
class InviteSection extends ConsumerStatefulWidget {
  const InviteSection({super.key, required this.group});

  final Group group;

  @override
  ConsumerState<InviteSection> createState() => _InviteSectionState();
}

class _InviteSectionState extends ConsumerState<InviteSection> {
  Invite? _created;
  bool _busy = false;

  String get _groupId => widget.group.id;

  /// The container outlives this section, so a change saved after the
  /// person left settings still refreshes the list of links.
  ProviderContainer get _container =>
      ProviderScope.containerOf(context, listen: false);

  Future<void> _create() async {
    final container = _container;
    setState(() => _busy = true);
    try {
      final invite = await container
          .read(invitesRepositoryProvider)
          .create(_groupId);
      container.invalidate(groupInvitesProvider(_groupId));
      if (mounted) setState(() => _created = invite);
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _copy(String url) async {
    await Clipboard.setData(ClipboardData(text: url));
    if (mounted) showMessage(context, 'Invite link copied');
  }

  Future<void> _share(String url) async {
    final how = await shareText(
      'Join “${widget.group.name}” on Split Spends to share expenses: $url',
      title: 'Join ${widget.group.name}',
    );
    if (mounted && how == Shared.clipboard) {
      showMessage(context, 'Invite copied. Paste it into a message.');
    }
  }

  Future<void> _revoke(Invite invite) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Revoke this link?'),
        content: const Text(
          'People who have it can no longer join. Members stay in the group.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Revoke'),
          ),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    final container = _container;
    try {
      await container
          .read(invitesRepositoryProvider)
          .revoke(_groupId, invite.id);
      container.invalidate(groupInvitesProvider(_groupId));
      if (!mounted) return;
      if (_created?.id == invite.id) setState(() => _created = null);
      showMessage(context, 'Link revoked');
    } catch (error) {
      if (mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    if (ref.watch(appModeProvider) != AppMode.connected) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: const [
          SectionHeader('Invite with a link'),
          ListTile(
            leading: Icon(Icons.link_off_rounded),
            title: Text('Invite links need a server'),
            subtitle: Text(
              'On this device only, add people by name instead. Connect to '
              'a server to share a link or QR code.',
            ),
          ),
        ],
      );
    }
    final invites = ref.watch(groupInvitesProvider(_groupId));
    final created = _created;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Invite with a link'),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
          child: Text(
            'Anyone with the link can join this group for 14 days. Scanning '
            'the QR code opens it on a phone.',
            style: text.bodyMedium,
          ),
        ),
        if (created?.url != null)
          _LinkCard(
            url: created!.url!,
            expiresAt: Dates.tryParse(created.expiresAt),
            onCopy: () => _copy(created.url!),
            onShare: () => _share(created.url!),
          ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
          child: Align(
            alignment: Alignment.centerLeft,
            child: FilledButton.tonalIcon(
              key: const Key('invite-create'),
              onPressed: _busy || widget.group.archived ? null : _create,
              icon: const Icon(Icons.add_link_rounded),
              label: Text(
                created == null ? 'Create invite link' : 'Create another link',
              ),
            ),
          ),
        ),
        ...switch (invites) {
          AsyncValue(:final value?) => [
            for (final invite in value)
              ListTile(
                key: ValueKey('invite-${invite.id}'),
                leading: const Icon(Icons.link_rounded),
                title: Text(
                  invite.id == created?.id
                      ? 'Link shown above'
                      : 'Link from ${_day(invite.createdAt)}',
                ),
                subtitle: Text('Expires ${_day(invite.expiresAt)}'),
                trailing: IconButton(
                  tooltip: 'Revoke link',
                  icon: const Icon(Icons.link_off_rounded),
                  onPressed: () => _revoke(invite),
                ),
              ),
          ],
          AsyncValue(:final error?) => [
            ListTile(
              leading: const Icon(Icons.error_outline_rounded),
              title: Text(describeError(error)),
            ),
          ],
          _ => const [LinearProgressIndicator()],
        },
      ],
    );
  }

  static String _day(String? stamp) {
    final date = Dates.tryParse(stamp);
    return date == null ? '' : Dates.short(date);
  }
}

class _LinkCard extends StatelessWidget {
  const _LinkCard({
    required this.url,
    required this.onCopy,
    required this.onShare,
    this.expiresAt,
  });

  final String url;
  final DateTime? expiresAt;
  final VoidCallback onCopy;
  final VoidCallback onShare;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final qr = Container(
      key: const Key('invite-qr'),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
      ),
      child: QrImageView(
        data: url,
        size: 168,
        padding: EdgeInsets.zero,
        backgroundColor: Colors.white,
        semanticsLabel: 'QR code for the invite link',
      ),
    );
    final details = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text('Your invite link', style: text.titleSmall),
        const SizedBox(height: 6),
        SelectableText(
          url,
          key: const Key('invite-url'),
          style: text.bodySmall?.copyWith(fontFamily: 'monospace'),
        ),
        if (expiresAt != null) ...[
          const SizedBox(height: 6),
          Text('Expires ${Dates.short(expiresAt!)}', style: text.bodySmall),
        ],
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            FilledButton.icon(
              key: const Key('invite-copy'),
              onPressed: onCopy,
              icon: const Icon(Icons.copy_rounded, size: 18),
              label: const Text('Copy'),
            ),
            OutlinedButton.icon(
              key: const Key('invite-share'),
              onPressed: onShare,
              icon: const Icon(Icons.share_rounded, size: 18),
              label: const Text('Share'),
            ),
          ],
        ),
      ],
    );
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 4, 16, 12),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: LayoutBuilder(
          builder: (context, constraints) => constraints.maxWidth >= 480
              ? Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    qr,
                    const SizedBox(width: 20),
                    Expanded(child: details),
                  ],
                )
              : Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Center(child: qr),
                    const SizedBox(height: 16),
                    details,
                  ],
                ),
        ),
      ),
    );
  }
}
