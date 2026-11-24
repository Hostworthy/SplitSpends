import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/groups/domain/group.dart';

final _emailPattern = RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$');

/// Opens the "new group" sheet; navigates to the group when created.
Future<void> showNewGroupSheet(BuildContext context) =>
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (_) => const _NewGroupSheet(),
    );

class _NewGroupSheet extends ConsumerStatefulWidget {
  const _NewGroupSheet();

  @override
  ConsumerState<_NewGroupSheet> createState() => _NewGroupSheetState();
}

class _NewGroupSheetState extends ConsumerState<_NewGroupSheet> {
  final _name = TextEditingController();
  final _email = TextEditingController();
  final _emails = <String>[];
  GroupKind _kind = GroupKind.trip;
  late String _currency = ref.read(meProvider)?.defaultCurrency ?? 'USD';
  bool _busy = false;

  @override
  void dispose() {
    _name.dispose();
    _email.dispose();
    super.dispose();
  }

  void _addEmail() {
    final email = _email.text.trim().toLowerCase();
    if (!_emailPattern.hasMatch(email) || _emails.contains(email)) return;
    setState(() {
      _emails.add(email);
      _email.clear();
    });
  }

  Future<void> _create() async {
    _addEmail();
    final name = _name.text.trim();
    if (name.isEmpty) return;
    setState(() => _busy = true);
    try {
      final group = await ref
          .read(groupsRepositoryProvider)
          .create(
            GroupInput(
              name: name,
              kind: _kind,
              defaultCurrency: _currency,
              simplifyDebts: true,
              memberEmails: _emails,
            ),
          );
      ref.refreshLedger();
      if (!mounted) return;
      Navigator.pop(context);
      GoRouter.of(context).go(Routes.group(group.id));
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return Padding(
      padding: EdgeInsets.only(
        left: 24,
        right: 24,
        bottom: MediaQuery.viewInsetsOf(context).bottom + 24,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text('New group', style: text.titleLarge),
          const SizedBox(height: 16),
          TextField(
            controller: _name,
            autofocus: true,
            textCapitalization: TextCapitalization.sentences,
            decoration: InputDecoration(
              labelText: 'Group name',
              prefixIcon: Padding(
                padding: const EdgeInsets.all(12),
                child: Text(_kind.emoji, style: const TextStyle(fontSize: 20)),
              ),
            ),
          ),
          const SizedBox(height: 12),
          Wrap(
            spacing: 8,
            children: [
              for (final kind in GroupKind.values)
                ChoiceChip(
                  label: Text('${kind.emoji}  ${kind.label}'),
                  selected: _kind == kind,
                  onSelected: (_) => setState(() => _kind = kind),
                ),
            ],
          ),
          const SizedBox(height: 12),
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: const Icon(Icons.currency_exchange_rounded),
            title: const Text('Default currency'),
            trailing: Text(_currency, style: text.titleMedium),
            onTap: () async {
              final code = await pickCurrency(context, selected: _currency);
              if (code != null) setState(() => _currency = code);
            },
          ),
          TextField(
            controller: _email,
            keyboardType: TextInputType.emailAddress,
            onSubmitted: (_) => _addEmail(),
            decoration: InputDecoration(
              labelText: 'Invite by email',
              helperText: 'People get an email invite. You can add more later.',
              suffixIcon: IconButton(
                icon: const Icon(Icons.add_rounded),
                onPressed: _addEmail,
              ),
            ),
          ),
          if (_emails.isNotEmpty) ...[
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [
                for (final e in _emails)
                  InputChip(
                    label: Text(e),
                    onDeleted: () => setState(() => _emails.remove(e)),
                  ),
              ],
            ),
          ],
          const SizedBox(height: 20),
          FilledButton(
            onPressed: _busy ? null : _create,
            child: const Text('Create group'),
          ),
        ],
      ),
    );
  }
}
