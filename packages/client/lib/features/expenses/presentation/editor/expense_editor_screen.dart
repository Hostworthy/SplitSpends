import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/widgets/back_home_button.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/editor/details_section.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/expenses/presentation/editor/itemized_section.dart';
import 'package:splitspends/features/expenses/presentation/editor/payers_section.dart';
import 'package:splitspends/features/expenses/presentation/editor/split_section.dart';
import 'package:splitspends/features/expenses/presentation/editor/with_section.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';
import 'package:splitspends/features/friends/presentation/people.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

/// Add or edit an expense. Resolves what it needs, then shows the form.
class ExpenseEditorScreen extends ConsumerWidget {
  const ExpenseEditorScreen({super.key, required this.args, this.expenseId});

  final ExpenseEditorArgs args;
  final String? expenseId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    Expense? existing = args.existing;
    if (existing == null && expenseId != null) {
      final loaded = ref.watch(expenseProvider(expenseId!));
      if (!loaded.hasValue) {
        return _Frame(
          title: 'Edit expense',
          child: AsyncView(
            value: loaded,
            onRetry: () => ref.invalidate(expenseProvider(expenseId!)),
            data: (_) => const SizedBox.shrink(),
          ),
        );
      }
      existing = loaded.requireValue;
    }
    final groupId = existing?.groupId ?? args.draft?.groupId ?? args.groupId;
    Group? group;
    if (groupId != null) {
      final loaded = ref.watch(groupProvider(groupId));
      if (!loaded.hasValue) {
        return _Frame(
          title: existing == null ? 'Add expense' : 'Edit expense',
          child: AsyncView(
            value: loaded,
            onRetry: () => ref.invalidate(groupProvider(groupId)),
            data: (_) => const SizedBox.shrink(),
          ),
        );
      }
      group = loaded.requireValue;
    }
    final me = ref.watch(meProvider);
    if (me == null) return const _Frame(title: '', child: SizedBox.shrink());
    // Make sure friends are loading so the "with" picker is ready.
    ref.watch(friendsProvider);
    return _EditorBody(
      key: ValueKey(existing?.id ?? 'new'),
      me: me,
      existing: existing,
      draft: args.draft,
      group: group,
      friendId: args.friendId,
    );
  }
}

class _Frame extends StatelessWidget {
  const _Frame({required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(title)),
    body: child,
  );
}

class _EditorBody extends ConsumerStatefulWidget {
  const _EditorBody({
    super.key,
    required this.me,
    this.existing,
    this.draft,
    this.group,
    this.friendId,
  });

  final Me me;
  final Expense? existing;
  final ExpenseInput? draft;
  final Group? group;
  final String? friendId;

  @override
  ConsumerState<_EditorBody> createState() => _EditorBodyState();
}

class _EditorBodyState extends ConsumerState<_EditorBody> {
  late final ExpenseForm form;
  bool _saving = false;

  /// Group members (when in a group) are always resolvable by id.
  final Map<String, User> _extraPeople = {};

  @override
  void initState() {
    super.initState();
    final group = widget.group;
    if (group != null) {
      for (final m in group.members) {
        _extraPeople[m.id] = m;
      }
    }
    form = ExpenseForm(
      meId: widget.me.id,
      currency: group?.defaultCurrency ?? widget.me.defaultCurrency,
      groupId: group?.id,
      people: group != null
          ? group.members.map((m) => m.id).toList()
          : [widget.me.id, ?widget.friendId],
    );
    final initial = widget.existing != null
        ? ExpenseInput.fromExpense(widget.existing!)
        : widget.draft;
    if (initial != null) form.load(initial);
  }

  @override
  void dispose() {
    form.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final problem = form.problem;
    if (problem != null) {
      showMessage(context, problem);
      return;
    }
    setState(() => _saving = true);
    try {
      final repo = ref.read(expensesRepositoryProvider);
      final input = form.toInput();
      final saved = widget.existing == null
          ? await repo.create(input)
          : await repo.update(widget.existing!.id, input);
      HapticFeedback.mediumImpact();
      ref.refreshLedger();
      if (!mounted) return;
      final router = GoRouter.of(context);
      router.pop();
      showMessage(
        context,
        widget.existing == null ? 'Expense added' : 'Expense updated',
        actionLabel: 'View',
        onAction: () => router.push(Routes.expense(saved.id)),
      );
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final people = {...ref.watch(peopleProvider), ..._extraPeople};
    return Scaffold(
      appBar: AppBar(
        leading: const BackHomeButton(close: true),
        title: Text(widget.existing == null ? 'Add expense' : 'Edit expense'),
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: 12),
            child: ListenableBuilder(
              listenable: form,
              builder: (context, _) => FilledButton(
                onPressed: _saving ? null : _save,
                style: FilledButton.styleFrom(minimumSize: const Size(80, 40)),
                child: _saving
                    ? const SizedBox.square(
                        dimension: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Text('Save'),
              ),
            ),
          ),
        ],
      ),
      body: CallbackShortcuts(
        bindings: {
          const SingleActivator(LogicalKeyboardKey.enter, control: true): _save,
          const SingleActivator(LogicalKeyboardKey.enter, meta: true): _save,
        },
        child: ListenableBuilder(
          listenable: form,
          builder: (context, _) => ListView(
            padding: const EdgeInsets.only(bottom: 48),
            children: [
              MaxWidth(
                width: 680,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (form.warnings.isNotEmpty) _Warnings(form.warnings),
                    WithSection(
                      form: form,
                      people: people,
                      onGroupMembers: (members) {
                        for (final m in members) {
                          _extraPeople[m.id] = m;
                        }
                      },
                    ),
                    DetailsSection(form: form, onSubmit: _save),
                    PayersSection(form: form, people: people),
                    if (form.method == SplitMethod.itemized)
                      ItemizedSection(form: form, people: people)
                    else
                      SplitSection(form: form, people: people),
                    _Problem(form: form),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Warnings extends StatelessWidget {
  const _Warnings(this.warnings);

  final List<String> warnings;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 8, 16, 0),
      color: scheme.tertiaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.auto_awesome_rounded, color: scheme.onTertiaryContainer),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                'Review the AI draft:\n${warnings.join('\n')}',
                style: TextStyle(color: scheme.onTertiaryContainer),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Problem extends StatelessWidget {
  const _Problem({required this.form});

  final ExpenseForm form;

  @override
  Widget build(BuildContext context) {
    final problem = form.problem;
    final scheme = Theme.of(context).colorScheme;
    return AnimatedSwitcher(
      duration: const Duration(milliseconds: 200),
      child: problem == null
          ? Padding(
              key: const ValueKey('ok'),
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Icon(Icons.check_circle_rounded, color: scheme.primary),
                  const SizedBox(width: 8),
                  const Text('Ready to save'),
                ],
              ),
            )
          : Padding(
              key: ValueKey(problem),
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Icon(Icons.info_outline_rounded, color: scheme.outline),
                  const SizedBox(width: 8),
                  Expanded(child: Text(problem)),
                ],
              ),
            ),
    );
  }
}
