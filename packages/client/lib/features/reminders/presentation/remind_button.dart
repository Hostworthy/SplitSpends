import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/utils/share.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/activity/presentation/providers.dart';
import 'package:splitspends/features/reminders/domain/reminder.dart';

/// Reminds [user] to settle what they owe you ([owed], positive amounts).
///
/// Connected: the server emails them. On this device: shares (or copies) a
/// text the user can send themselves. People without an email cannot get
/// reminders from a server, so the button is disabled with an explanation.
class RemindButton extends ConsumerStatefulWidget {
  const RemindButton({
    super.key,
    required this.user,
    required this.owed,
    this.groupId,
    this.groupName,
    this.compact = true,
  });

  final User user;
  final List<Balance> owed;
  final String? groupId;
  final String? groupName;

  /// An icon button (lists) instead of a labelled button (headers).
  final bool compact;

  @override
  ConsumerState<RemindButton> createState() => _RemindButtonState();
}

class _RemindButtonState extends ConsumerState<RemindButton> {
  bool _busy = false;

  String get _name => widget.user.firstName;

  Future<void> _send() async {
    setState(() => _busy = true);
    try {
      final sent = await ref
          .read(remindersRepositoryProvider)
          .send(widget.user.id, groupId: widget.groupId);
      ref.invalidate(activityFeedProvider);
      if (!mounted) return;
      showMessage(
        context,
        sent.emailed
            ? 'Reminder sent to $_name'
            : '$_name turned reminder emails off. They will see it in '
                  'their activity.',
      );
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _share() async {
    final config = ref.read(configValueProvider);
    final text = reminderText(
      name: _name,
      amounts: [
        for (final b in widget.owed)
          Money.format(b.amount, config.currency(b.currency), absolute: true),
      ],
      group: widget.groupName,
    );
    final how = await shareText(text, title: 'Settle up reminder');
    if (mounted && how == Shared.clipboard) {
      showMessage(
        context,
        'Reminder copied. Paste it into a message to $_name.',
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final local = ref.watch(appModeProvider) == AppMode.local;
    final reachable = local || widget.user.email != null;
    final String tip;
    if (!reachable) {
      tip = '$_name has no email address yet, so they cannot get reminders';
    } else if (local) {
      tip = 'Share a reminder with $_name';
    } else {
      tip = 'Remind $_name to settle up';
    }
    final VoidCallback? onPressed = !reachable || _busy
        ? null
        : (local ? _share : _send);
    final icon = Icon(
      local ? Icons.share_outlined : Icons.notifications_active_outlined,
    );
    final Widget button = widget.compact
        ? IconButton(
            key: ValueKey('remind-${widget.user.id}'),
            onPressed: onPressed,
            icon: icon,
          )
        : OutlinedButton.icon(
            key: ValueKey('remind-${widget.user.id}'),
            onPressed: onPressed,
            icon: icon,
            label: Text(local ? 'Share reminder' : 'Remind'),
          );
    // Wrapping keeps the explanation visible when the button is disabled.
    return Tooltip(message: tip, child: button);
  }
}
