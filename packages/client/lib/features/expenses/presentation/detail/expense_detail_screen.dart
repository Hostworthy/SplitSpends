import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/widgets/back_home_button.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/features/categories/presentation/category_badge.dart';
import 'package:splitspends/features/categories/presentation/providers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/detail/comments_section.dart';
import 'package:splitspends/features/expenses/presentation/detail/history_section.dart';
import 'package:splitspends/features/expenses/presentation/detail/receipt_image.dart';
import 'package:splitspends/features/expenses/presentation/detail/shares_card.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';
import 'package:splitspends/features/friends/presentation/people.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

class ExpenseDetailScreen extends ConsumerWidget {
  const ExpenseDetailScreen({super.key, required this.expenseId});

  final String expenseId;

  Future<void> _delete(BuildContext context, WidgetRef ref) async {
    final repo = ref.read(expensesRepositoryProvider);
    final messenger = ScaffoldMessenger.of(context);
    // Undo runs after this screen is gone, when its ref can no longer be
    // used; the container outlives it.
    final container = ProviderScope.containerOf(context, listen: false);
    try {
      await repo.delete(expenseId);
      unawaited(HapticFeedback.mediumImpact());
      refreshLedgerIn(container);
      if (context.mounted) context.pop();
      messenger.showSnackBar(
        SnackBar(
          content: const Text('Expense deleted'),
          duration: const Duration(seconds: 6),
          action: SnackBarAction(
            label: 'Undo',
            onPressed: () async {
              try {
                await repo.restore(expenseId);
                refreshLedgerIn(container);
              } catch (error) {
                messenger.showSnackBar(
                  SnackBar(content: Text(describeError(error))),
                );
              }
            },
          ),
        ),
      );
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  Future<void> _restore(BuildContext context, WidgetRef ref) async {
    final container = ProviderScope.containerOf(context, listen: false);
    try {
      await ref.read(expensesRepositoryProvider).restore(expenseId);
      refreshLedgerIn(container);
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final expense = ref.watch(expenseProvider(expenseId));
    final e = expense.value;
    return Scaffold(
      appBar: AppBar(
        leading: const BackHomeButton(),
        actions: [
          if (e != null && !e.deleted && !e.isPayment)
            IconButton(
              tooltip: e.editable
                  ? 'Edit'
                  : 'Update the app to edit this expense',
              icon: const Icon(Icons.edit_rounded),
              onPressed: e.editable
                  ? () => context.push(
                      Routes.editExpense(expenseId),
                      extra: ExpenseEditorArgs(existing: e),
                    )
                  : null,
            ),
          if (e != null && !e.deleted)
            IconButton(
              tooltip: 'Delete',
              icon: const Icon(Icons.delete_outline_rounded),
              onPressed: () => _delete(context, ref),
            ),
        ],
      ),
      body: AsyncView(
        value: expense,
        onRetry: () => ref.invalidate(expenseProvider(expenseId)),
        data: (e) => RefreshIndicator(
          onRefresh: () async {
            ref.invalidate(expenseProvider(expenseId));
            ref.invalidate(commentsProvider(expenseId));
            await ref.read(expenseProvider(expenseId).future);
          },
          child: ListView(
            padding: const EdgeInsets.only(bottom: 48),
            children: [
              MaxWidth(
                width: 680,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (e.deleted)
                      MaterialBanner(
                        content: const Text('This expense was deleted.'),
                        actions: [
                          TextButton(
                            onPressed: () => _restore(context, ref),
                            child: const Text('Restore'),
                          ),
                        ],
                      ),
                    _Header(expense: e),
                    SharesCard(expense: e),
                    if (e.notes.isNotEmpty)
                      Card(
                        margin: const EdgeInsets.fromLTRB(16, 12, 16, 0),
                        child: ListTile(
                          leading: const Icon(Icons.notes_rounded),
                          title: Text(e.notes),
                        ),
                      ),
                    if (e.receiptId != null)
                      ReceiptImage(receiptId: e.receiptId!),
                    CommentsSection(expenseId: e.id),
                    HistorySection(expenseId: e.id),
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

class _Header extends ConsumerWidget {
  const _Header({required this.expense});

  /// A save this soon after adding counts as part of adding the expense,
  /// not as an edit worth showing.
  static const _settling = Duration(seconds: 5);

  final Expense expense;

  /// "Added by you 2 days ago · edited by Ben 1 hour ago", or null when the
  /// server did not say when the expense was added.
  String? _history(Map<String, User> people, String? meId) {
    final created = Dates.tryParse(expense.createdAt);
    if (created == null) return null;
    String who(String id) => id == meId ? 'you' : people.name(id, meId: meId);
    final added =
        'Added by ${who(expense.createdBy ?? '')} ${Dates.relative(created)}';
    final updated = Dates.tryParse(expense.updatedAt);
    final editor = expense.updatedBy;
    if (updated == null ||
        editor == null ||
        updated.difference(created) <= _settling) {
      return added;
    }
    return '$added · edited by ${who(editor)} ${Dates.relative(updated)}';
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(expense.currency);
    final people = ref.watch(peopleProvider);
    final meId = ref.watch(meProvider)?.id;
    final text = Theme.of(context).textTheme;
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    final groupName = expense.groupId == null
        ? null
        : ref.watch(groupProvider(expense.groupId!)).value?.name;
    final history = _history(people, meId);

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Hero(
            tag: 'expense-icon-${expense.id}',
            child: CategoryBadge(
              category: expense.category,
              payment: expense.isPayment,
              size: 64,
            ),
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  expense.isPayment ? 'Payment' : expense.description,
                  style: text.titleLarge,
                ),
                Text(
                  Money.format(expense.total, info),
                  style: text.headlineMedium,
                ),
                const SizedBox(height: 4),
                Text(
                  [
                    DateFormat.yMMMMEEEEd().format(expense.day),
                    ?groupName,
                    if (!expense.isPayment)
                      ref
                          .watch(categoryCatalogProvider)
                          .find(expense.category)
                          .name,
                  ].where((s) => s.isNotEmpty).join(' · '),
                  style: text.bodyMedium?.copyWith(color: muted),
                ),
                if (history != null)
                  Text(history, style: text.bodySmall?.copyWith(color: muted)),
                if (expense.unknownRecurrence != null)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Chip(
                      avatar: const Icon(Icons.repeat_rounded, size: 16),
                      label: Text('Repeats: ${expense.unknownRecurrence}'),
                    ),
                  )
                else if (expense.recurrence != Recurrence.none)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Chip(
                      avatar: const Icon(Icons.repeat_rounded, size: 16),
                      label: Text(
                        'Repeats ${expense.recurrence.label.toLowerCase()}',
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
