import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/editor/split_section.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/default_split_form.dart';

/// Group settings → "Default split": the method and per-member values new
/// expenses in this group start with, validated live.
class DefaultSplitSection extends ConsumerStatefulWidget {
  const DefaultSplitSection({super.key, required this.group});

  final Group group;

  @override
  ConsumerState<DefaultSplitSection> createState() =>
      _DefaultSplitSectionState();
}

class _DefaultSplitSectionState extends ConsumerState<DefaultSplitSection> {
  late DefaultSplitForm _form = _build();
  bool _saving = false;

  DefaultSplitForm _build() => DefaultSplitForm(
    members: [for (final m in widget.group.members) m.id],
    initial: widget.group.defaultSplit,
  );

  @override
  void didUpdateWidget(DefaultSplitSection old) {
    super.didUpdateWidget(old);
    final members = [for (final m in widget.group.members) m.id];
    if (members.join(',') != _form.members.join(',')) {
      _form.dispose();
      _form = _build();
    } else if (!identical(old.group, widget.group) && !_form.changed) {
      _form.load(widget.group.defaultSplit);
    }
  }

  @override
  void dispose() {
    _form.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final split = _form.split;
    setState(() => _saving = true);
    try {
      final saved = await ref
          .read(groupsRepositoryProvider)
          .update(
            widget.group.id,
            GroupInput(defaultSplit: split, clearDefaultSplit: split == null),
          );
      _form.load(saved.defaultSplit);
      ref.refreshLedger();
      if (mounted) {
        showMessage(
          context,
          split == null ? 'Default split removed' : 'Default split saved',
        );
      }
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final meId = ref.watch(meProvider)?.id;
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    return ListenableBuilder(
      listenable: _form,
      builder: (context, _) {
        final method = _form.method;
        final problem = _form.problem;
        final status = problem ?? _form.remaining;
        return Column(
          key: const Key('default-split'),
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SectionHeader('Default split'),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
              child: Text(
                'New expenses in this group start with this split. You can '
                'still change it on each expense.',
                style: text.bodyMedium?.copyWith(
                  color: scheme.onSurfaceVariant,
                ),
              ),
            ),
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: SegmentedButton<SplitMethod?>(
                showSelectedIcon: false,
                segments: [
                  const ButtonSegment(value: null, label: Text('None')),
                  for (final m in DefaultSplit.methods)
                    ButtonSegment(value: m, label: Text(m.label)),
                ],
                selected: {method},
                onSelectionChanged: (v) => _form.setMethod(v.first),
              ),
            ),
            if (method == SplitMethod.percent || method == SplitMethod.shares)
              for (final m in widget.group.members)
                ListTile(
                  leading: UserAvatar(user: m),
                  title: Text(m.id == meId ? 'You' : m.displayName),
                  trailing: SizedBox(
                    width: 120,
                    child: DecimalField(
                      key: ValueKey('default-${method!.name}-${m.id}'),
                      initial: _form.values[m.id],
                      suffix: method == SplitMethod.percent ? '%' : 'shares',
                      onChanged: (v) => _form.setValue(m.id, v),
                    ),
                  ),
                ),
            if (method == SplitMethod.equal)
              const ListTile(
                leading: Icon(Icons.groups_rounded),
                title: Text('Everyone in the group, equally'),
              ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
              child: Row(
                children: [
                  if (status != null) ...[
                    Icon(
                      problem == null
                          ? Icons.check_circle_rounded
                          : Icons.info_outline_rounded,
                      size: 18,
                      color: problem == null ? scheme.primary : scheme.error,
                    ),
                    const SizedBox(width: 6),
                    Expanded(
                      child: Text(
                        status,
                        style: text.labelLarge?.copyWith(
                          color: problem == null
                              ? scheme.primary
                              : scheme.error,
                        ),
                      ),
                    ),
                  ] else
                    const Spacer(),
                  FilledButton.tonal(
                    key: const Key('save-default-split'),
                    onPressed: _saving || !_form.canSave ? null : _save,
                    child: const Text('Save'),
                  ),
                ],
              ),
            ),
          ],
        );
      },
    );
  }
}
