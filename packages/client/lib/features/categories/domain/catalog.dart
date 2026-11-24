import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/widgets/category_icon.dart';

/// Shown for ids that are not in the catalog, such as a deleted custom
/// category: its expenses show as General without being rewritten.
const generalCategory = Category(
  id: 'general',
  name: 'General',
  group: 'Other',
  icon: 'receipt_long',
);

const paymentCategoryId = 'payment';

/// Built-in plus custom categories as one lookup.
class CategoryCatalog {
  CategoryCatalog(List<Category> all)
    : all = List.unmodifiable(all),
      _index = {for (final c in all) c.id: c};

  final List<Category> all;
  final Map<String, Category> _index;

  /// The category with [id], or General when it is unknown or deleted.
  Category find(String? id) =>
      _index[id] ?? _index[generalCategory.id] ?? generalCategory;

  /// Your own custom categories, by name.
  List<Category> get mine => [
    for (final c in all)
      if (c.custom && c.mine) c,
  ]..sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));

  /// What a new expense may use: built-ins (except payment) and your own,
  /// plus [selected] when an edited expense already has another one.
  List<Category> choices({String? selected}) => [
    for (final c in all)
      if (c.id != paymentCategoryId &&
          (!c.custom || c.mine || c.id == selected))
        c,
  ];

  /// Display groups in order: those of the built-in categories.
  List<String> get groups {
    final out = <String>[];
    for (final c in all) {
      if (!c.custom && !out.contains(c.group)) out.add(c.group);
    }
    if (!out.contains(generalCategory.group)) out.add(generalCategory.group);
    return out;
  }
}

/// Name, icon and display group of a custom category being edited. Pure
/// Dart so the rules (the server's) are easy to test.
class CategoryDraft {
  CategoryDraft({this.name = '', String? icon, String? group})
    : icon = icon ?? 'label',
      group = group ?? generalCategory.group;

  static const maxName = 40;

  String name;
  String icon;
  String group;

  String get cleanName =>
      name.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).join(' ');

  /// The first problem preventing save, or null.
  String? problem(List<String> groups) {
    final n = cleanName;
    if (n.isEmpty) return 'Give the category a name';
    if (n.runes.length > maxName) {
      return 'Category names are limited to $maxName characters';
    }
    if (!categoryIcons.containsKey(icon)) return 'Choose an icon';
    if (!groups.contains(group)) return 'Choose a group';
    return null;
  }
}
