import 'dart:async';

import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/api/address.dart';
import 'package:splitspends/features/invites/domain/invite.dart';

/// Asks for a pasted invite link and opens it.
Future<void> showJoinLinkDialog(BuildContext context) async {
  final link = await showDialog<JoinLink>(
    context: context,
    builder: (context) => const _JoinLinkDialog(),
  );
  if (link != null && context.mounted) {
    unawaited(context.push(Routes.join(link.token, server: link.server)));
  }
}

class _JoinLinkDialog extends StatefulWidget {
  const _JoinLinkDialog();

  @override
  State<_JoinLinkDialog> createState() => _JoinLinkDialogState();
}

class _JoinLinkDialogState extends State<_JoinLinkDialog> {
  final _controller = TextEditingController();
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _submit() {
    final link = JoinLink.parse(_controller.text);
    if (link == null) {
      setState(
        () => _error = publicPlainServer(_controller.text)
            ? 'This link starts with http://, which is only safe on your '
                  'local network. Ask for a link that starts with https://.'
            : 'Paste the whole invite link, for example '
                  'https://split.example.com/join/…',
      );
      return;
    }
    Navigator.pop(context, link);
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: const Text('Join with a link'),
    content: TextField(
      key: const Key('join-link-field'),
      controller: _controller,
      autofocus: true,
      keyboardType: TextInputType.url,
      textInputAction: TextInputAction.go,
      onSubmitted: (_) => _submit(),
      decoration: InputDecoration(
        labelText: 'Invite link',
        prefixIcon: const Icon(Icons.link_rounded),
        errorText: _error,
        errorMaxLines: 3,
      ),
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.pop(context),
        child: const Text('Cancel'),
      ),
      FilledButton(onPressed: _submit, child: const Text('Open')),
    ],
  );
}
