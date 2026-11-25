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

/// A custom category id: `custom.` and a lowercase canonical UUID, as the
/// server requires.
final customId = RegExp(
  r'^custom\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
);

/// Name, icon and display group of a custom category being edited. Pure
/// Dart so the rules (the server's) are easy to test. A blank icon or group
/// is the default one, as on the server.
class CategoryDraft {
  CategoryDraft({this.name = '', String? icon, String? group})
    : icon = _filled(icon, 'label'),
      group = _filled(group, generalCategory.group);

  static const maxName = 40;

  static String _filled(String? value, String fallback) {
    final trimmed = value?.trim() ?? '';
    return trimmed.isEmpty ? fallback : trimmed;
  }

  String name;
  String icon;
  String group;

  String get cleanName =>
      name.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).join(' ');

  /// The first problem preventing save, with the API field it concerns and
  /// the server's message, or null.
  ({String field, String message})? problem(List<String> groups) {
    final n = cleanName;
    if (n.isEmpty) {
      return (field: 'name', message: 'Give the category a name.');
    }
    if (n.runes.length > maxName) {
      return (
        field: 'name',
        message: 'Category names are limited to $maxName characters.',
      );
    }
    if (!categoryIcons.containsKey(icon)) {
      return (field: 'icon', message: 'Choose one of the offered icons.');
    }
    if (!groups.contains(group)) {
      return (field: 'group', message: 'Choose one of the category groups.');
    }
    return null;
  }
}
