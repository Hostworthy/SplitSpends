import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/refresh.dart';
import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/widgets/back_home_button.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/rates/rates.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/category_icon.dart';
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
    try {
      await repo.delete(expenseId);
      HapticFeedback.mediumImpact();
      ref.refreshLedger();
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
                ref.refreshLedger();
              } catch (_) {
                messenger.showSnackBar(
                  const SnackBar(content: Text('Could not restore')),
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
    try {
      await ref.read(expensesRepositoryProvider).restore(expenseId);
      ref.refreshLedger();
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
              tooltip: 'Edit',
              icon: const Icon(Icons.edit_rounded),
              onPressed: () => context.push(
                Routes.editExpense(expenseId),
                extra: ExpenseEditorArgs(existing: e),
              ),
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

  final Expense expense;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final info = ref.watch(configValueProvider).currency(expense.currency);
    final config = ref.watch(configValueProvider);
    final view = ref.watch(moneyViewProvider);
    final people = ref.watch(peopleProvider);
    final meId = ref.watch(meProvider)?.id;
    final text = Theme.of(context).textTheme;
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    final groupName = expense.groupId == null
        ? null
        : ref.watch(groupProvider(expense.groupId!)).value?.name;
    final created = Dates.tryParse(expense.createdAt);
    final updated = Dates.tryParse(expense.updatedAt);

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Hero(
            tag: 'expense-icon-${expense.id}',
            child: CategoryIcon(
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
                if (view.original(expense.total, expense.currency) != null)
                  Text(
                    'About ${view.format(expense.total, expense.currency)} '
                    'at today\'s rate',
                    style: text.bodyMedium?.copyWith(color: muted),
                  ),
                const SizedBox(height: 4),
                Text(
                  [
                    DateFormat.yMMMMEEEEd().format(expense.day),
                    ?groupName,
                    if (!expense.isPayment)
                      config.category(expense.category)?.name ?? '',
                  ].where((s) => s.isNotEmpty).join(' · '),
                  style: text.bodyMedium?.copyWith(color: muted),
                ),
                if (created != null)
                  Text(
                    'Added by ${expense.createdBy == meId ? 'you' : people.nameOf(expense.createdBy ?? '', meId: meId)} '
                    '${Dates.relative(created)}'
                    '${updated != null && expense.updatedBy != null && updated.difference(created).inSeconds > 5 ? ' · edited by ${expense.updatedBy == meId ? 'you' : people.nameOf(expense.updatedBy!, meId: meId)} ${Dates.relative(updated)}' : ''}',
                    style: text.bodySmall?.copyWith(color: muted),
                  ),
                if (expense.recurrence != Recurrence.none)
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
