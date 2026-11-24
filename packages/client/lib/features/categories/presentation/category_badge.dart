import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/widgets/category_icon.dart';
import 'package:splitspends/features/categories/presentation/providers.dart';

/// [CategoryIcon] for a category id, resolving custom categories (their
/// icon and group) through the catalog. Unknown or deleted ids show as
/// General.
class CategoryBadge extends ConsumerWidget {
  const CategoryBadge({
    super.key,
    required this.category,
    this.payment = false,
    this.size = 44,
  });

  final String? category;
  final bool payment;
  final double size;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = ref.watch(categoryCatalogProvider).find(category);
    return CategoryIcon(
      category: c.id,
      payment: payment,
      size: size,
      icon: c.icon,
      group: c.group,
    );
  }
}
