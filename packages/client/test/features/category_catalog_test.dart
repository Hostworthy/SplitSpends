import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/core/widgets/category_icon.dart';
import 'package:splitspends/features/categories/domain/catalog.dart';

void main() {
  const mine = Category(
    id: 'custom.11111111-1111-4111-8111-111111111111',
    name: 'Climbing',
    group: 'Entertainment',
    icon: 'fitness_center',
    custom: true,
    mine: true,
  );
  const theirs = Category(
    id: 'custom.22222222-2222-4222-8222-222222222222',
    name: 'Plants',
    group: 'Home',
    icon: 'park',
    custom: true,
  );
  final catalog = CategoryCatalog([...localConfig.categories, theirs, mine]);

  test('finds built-in and custom categories, General otherwise', () {
    expect(catalog.find('food.dining').name, 'Dining out');
    expect(catalog.find(mine.id).name, 'Climbing');
    expect(catalog.find(theirs.id).name, 'Plants');
    expect(catalog.find('custom.deleted').name, 'General');
    expect(catalog.find(null).id, 'general');
  });

  test('picker offers built-ins and your own, not payment', () {
    final ids = catalog.choices().map((c) => c.id).toSet();
    expect(ids, contains('food.dining'));
    expect(ids, contains(mine.id));
    expect(ids, isNot(contains(theirs.id)));
    expect(ids, isNot(contains('payment')));
    expect(
      catalog.choices(selected: theirs.id).map((c) => c.id),
      contains(theirs.id),
      reason: 'an edited expense keeps showing its category',
    );
    expect(catalog.mine.map((c) => c.id), [mine.id]);
  });

  test('display groups follow the built-in order', () {
    expect(catalog.groups.first, 'Food and drink');
    expect(catalog.groups.last, 'Other');
  });

  test('custom icons render with the group tint', () {
    final v = categoryVisual(mine.id, icon: mine.icon, group: mine.group);
    expect(v.icon, categoryIcons['fitness_center']);
    expect(v.color, categoryVisual('entertainment.movies').color);
  });

  test('every built-in icon is offered for custom categories', () {
    for (final c in localConfig.categories) {
      if (c.id == 'payment') continue;
      expect(categoryIcons.keys, contains(c.icon), reason: c.id);
    }
  });

  group('CategoryDraft', () {
    final groups = catalog.groups;

    test('names are 1 to 40 characters, collapsed', () {
      final draft = CategoryDraft(name: '   ');
      expect(draft.problem(groups)?.message, 'Give the category a name.');
      draft.name = '  Board   games ';
      expect(draft.problem(groups), isNull);
      expect(draft.cleanName, 'Board games');
      draft.name = 'x' * 41;
      expect(draft.problem(groups)?.message, contains('40 characters'));
    });

    test('icon and group must be offered ones', () {
      expect(
        CategoryDraft(name: 'A', icon: 'rocket').problem(groups)?.message,
        'Choose one of the offered icons.',
      );
      expect(
        CategoryDraft(name: 'A', group: 'Hobbies').problem(groups)?.message,
        'Choose one of the category groups.',
      );
    });

    test('problems carry the API field', () {
      expect(CategoryDraft().problem(groups), (
        field: 'name',
        message: 'Give the category a name.',
      ));
      expect(CategoryDraft(name: 'x' * 41).problem(groups), (
        field: 'name',
        message: 'Category names are limited to 40 characters.',
      ));
      expect(
        CategoryDraft(name: 'A', icon: 'rocket').problem(groups)?.field,
        'icon',
      );
      expect(
        CategoryDraft(name: 'A', group: 'Hobbies').problem(groups)?.field,
        'group',
      );
    });

    test('a blank icon or group is the default', () {
      final draft = CategoryDraft(name: 'A', icon: ' ', group: '');
      expect(draft.icon, 'label');
      expect(draft.group, 'Other');
      expect(draft.problem(groups), isNull);
    });

    test('custom ids look like custom.<uuid>', () {
      expect(customId.hasMatch(mine.id), isTrue);
      for (final id in [
        'custom.',
        'custom.nope',
        'custom.11111111111141118111111111111111',
        'custom.11111111-1111-4111-8111-11111111111A',
        'food.11111111-1111-4111-8111-111111111111',
      ]) {
        expect(customId.hasMatch(id), isFalse, reason: id);
      }
    });
  });
}
