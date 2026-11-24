import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/activity/presentation/activity_tile.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';

/// Who created, edited, deleted or commented on this expense.
class HistorySection extends ConsumerWidget {
  const HistorySection({super.key, required this.expenseId});

  final String expenseId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final history = ref.watch(expenseHistoryProvider(expenseId));
    final items = history.value ?? const [];
    if (items.isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('History'),
        for (final a in items) ActivityTile(activity: a, linkable: false),
      ],
    );
  }
}
