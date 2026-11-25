import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/routes.dart';
import 'package:splitspends/features/expenses/presentation/route_args.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';
import 'package:splitspends/features/receipts/data/providers.dart';

/// "Dinner 60 paid by me split with Sam" → draft → editor.
Future<void> showAiQuickAddSheet(BuildContext context, {String? groupId}) =>
    showModalBottomSheet<void>(
      useRootNavigator: true,
      context: context,
      isScrollControlled: true,
      builder: (_) => _AiQuickAddSheet(groupId: groupId),
    );

class _AiQuickAddSheet extends ConsumerStatefulWidget {
  const _AiQuickAddSheet({this.groupId});

  final String? groupId;

  @override
  ConsumerState<_AiQuickAddSheet> createState() => _AiQuickAddSheetState();
}

class _AiQuickAddSheetState extends ConsumerState<_AiQuickAddSheet> {
  final _text = TextEditingController();
  late String? _groupId = widget.groupId;
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _text.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final text = _text.text.trim();
    if (text.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final draft = await ref
          .read(receiptsRepositoryProvider)
          .draft(text, groupId: _groupId);
      if (!mounted) return;
      final router = GoRouter.of(context);
      Navigator.pop(context);
      unawaited(
        router.push(
          Routes.newExpense,
          extra: ExpenseEditorArgs(groupId: draft.groupId, draft: draft),
        ),
      );
    } catch (error) {
      if (!mounted) return;
      setState(
        () => _error = error is ApiError && error.isAiDisabled
            ? 'AI is not enabled on this server.'
            : describeError(error),
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final groups = ref.watch(groupsProvider).value ?? const <Group>[];
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
          Row(
            children: [
              Icon(
                Icons.auto_awesome_rounded,
                color: Theme.of(context).colorScheme.primary,
              ),
              const SizedBox(width: 8),
              Text('Describe the expense', style: text.titleLarge),
            ],
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _text,
            autofocus: true,
            minLines: 2,
            maxLines: 4,
            textInputAction: TextInputAction.send,
            onSubmitted: (_) => _submit(),
            decoration: InputDecoration(
              hintText: 'Dinner 60 paid by me, split with Sam and Alex',
              errorText: _error,
            ),
          ),
          const SizedBox(height: 12),
          DropdownButtonFormField<String?>(
            initialValue: _groupId,
            decoration: const InputDecoration(labelText: 'Group'),
            items: [
              const DropdownMenuItem(value: null, child: Text('No group')),
              for (final g in groups)
                DropdownMenuItem(value: g.id, child: Text(g.name)),
            ],
            onChanged: (v) => setState(() => _groupId = v),
          ),
          const SizedBox(height: 8),
          Text(
            'AI prepares a draft. Nothing is saved until you review it.',
            style: text.bodySmall,
          ),
          const SizedBox(height: 16),
          FilledButton.icon(
            onPressed: _busy ? null : _submit,
            icon: _busy
                ? const SizedBox.square(
                    dimension: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.arrow_forward_rounded),
            label: const Text('Create draft'),
          ),
        ],
      ),
    );
  }
}
