import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';

final _emailPattern = RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$');

/// Result of [showPersonDialog]: at least one of the two is set.
typedef PersonEntry = ({String? name, String? email});

/// Asks for a person by name, email or both. With [emailOnly], asks just for
/// an email (adding one to someone you added by name).
Future<PersonEntry?> showPersonDialog(
  BuildContext context, {
  required String title,
  String action = 'Add',
  bool emailOnly = false,
  String? initialName,
}) => showDialog<PersonEntry>(
  context: context,
  builder: (_) => _PersonDialog(
    title: title,
    action: action,
    emailOnly: emailOnly,
    initialName: initialName,
  ),
);

class _PersonDialog extends ConsumerStatefulWidget {
  const _PersonDialog({
    required this.title,
    required this.action,
    required this.emailOnly,
    this.initialName,
  });

  final String title;
  final String action;
  final bool emailOnly;
  final String? initialName;

  @override
  ConsumerState<_PersonDialog> createState() => _PersonDialogState();
}

class _PersonDialogState extends ConsumerState<_PersonDialog> {
  late final _name = TextEditingController(text: widget.initialName);
  final _email = TextEditingController();
  String? _error;

  @override
  void dispose() {
    _name.dispose();
    _email.dispose();
    super.dispose();
  }

  void _submit() {
    final name = _name.text.trim();
    final email = _email.text.trim().toLowerCase();
    if (email.isNotEmpty && !_emailPattern.hasMatch(email)) {
      setState(() => _error = 'Enter a valid email');
      return;
    }
    if (widget.emailOnly && email.isEmpty) {
      setState(() => _error = 'Enter an email');
      return;
    }
    if (name.isEmpty && email.isEmpty) {
      setState(() => _error = 'Enter a name or an email');
      return;
    }
    Navigator.pop<PersonEntry>(context, (
      name: name.isEmpty ? null : name,
      email: email.isEmpty ? null : email,
    ));
  }

  @override
  Widget build(BuildContext context) {
    final local = ref.watch(appModeProvider) == AppMode.local;
    final hint = widget.emailOnly
        ? (local
              ? 'Saved on this device; used to match them if you connect to a server later.'
              : 'We will email them an invite to join.')
        : (local
              ? 'Email is optional. It helps match people if you connect to a server later.'
              : 'Add by name; add their email later to invite them.');
    return AlertDialog(
      title: Text(widget.title),
      content: SizedBox(
        width: 380,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (!widget.emailOnly) ...[
              TextField(
                key: const Key('person-name'),
                controller: _name,
                autofocus: true,
                textCapitalization: TextCapitalization.words,
                textInputAction: TextInputAction.next,
                decoration: const InputDecoration(labelText: 'Name'),
              ),
              const SizedBox(height: 12),
            ],
            TextField(
              key: const Key('person-email'),
              controller: _email,
              autofocus: widget.emailOnly,
              keyboardType: TextInputType.emailAddress,
              onSubmitted: (_) => _submit(),
              decoration: InputDecoration(
                labelText: widget.emailOnly ? 'Email' : 'Email (optional)',
                errorText: _error,
              ),
            ),
            const SizedBox(height: 10),
            Text(hint, style: Theme.of(context).textTheme.bodySmall),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          key: const Key('person-submit'),
          onPressed: _submit,
          child: Text(widget.action),
        ),
      ],
    );
  }
}
