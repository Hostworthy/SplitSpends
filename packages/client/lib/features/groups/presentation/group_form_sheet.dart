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
      useRootNavigator: true,
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
  final _person = TextEditingController();
  final _personFocus = FocusNode();
  final _members = <MemberInput>[];
  GroupKind _kind = GroupKind.trip;
  late String _currency = ref.read(meProvider)?.defaultCurrency ?? 'USD';
  bool _busy = false;

  @override
  void dispose() {
    _name.dispose();
    _person.dispose();
    _personFocus.dispose();
    super.dispose();
  }

  /// Text with an `@` is an email; anything else is a name.
  void _addPerson() {
    final text = _person.text.trim();
    if (text.isEmpty) return;
    final isEmail = _emailPattern.hasMatch(text);
    final member = isEmail
        ? MemberInput(email: text.toLowerCase())
        : MemberInput(name: text);
    if (_members.any(
      (m) => m.label.toLowerCase() == member.label.toLowerCase(),
    )) {
      _person.clear();
      return;
    }
    setState(() {
      _members.add(member);
      _person.clear();
    });
  }

  Future<void> _create() async {
    _addPerson();
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
              emoji: _kind.emoji,
              defaultCurrency: _currency,
              simplifyDebts: true,
              members: _members,
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
            title: const Text('Currency'),
            subtitle: const Text('Every expense in this group uses it.'),
            trailing: Text(_currency, style: text.titleMedium),
            onTap: () async {
              final code = await pickCurrency(context, selected: _currency);
              if (code != null) setState(() => _currency = code);
            },
          ),
          TextField(
            controller: _person,
            focusNode: _personFocus,
            textInputAction: TextInputAction.done,
            // Handling onEditingComplete keeps focus in the field, so
            // several people can be added in a row.
            onEditingComplete: _addPerson,
            decoration: InputDecoration(
              labelText: 'Add people by name or email',
              helperText: ref.watch(appModeProvider) == AppMode.local
                  ? 'Names are enough. You can add more later.'
                  : 'People with an email get an invite. Add others by name.',
              suffixIcon: IconButton(
                icon: const Icon(Icons.add_rounded),
                onPressed: _addPerson,
              ),
            ),
          ),
          if (_members.isNotEmpty) ...[
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [
                for (final m in _members)
                  InputChip(
                    avatar: Icon(
                      m.email == null
                          ? Icons.person_outline_rounded
                          : Icons.alternate_email_rounded,
                      size: 18,
                    ),
                    label: Text(m.label),
                    onDeleted: () => setState(() => _members.remove(m)),
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
