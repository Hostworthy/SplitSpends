import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/categories/data/local_categories_repository.dart';
import 'package:splitspends/features/categories/domain/categories_repository.dart';
import 'package:splitspends/features/expenses/data/local_expenses_repository.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/sync/domain/import_payload.dart';

void main() {
  late LocalLedger ledger;
  const meId = '00000000-0000-4000-8000-00000000000a';
  const me = Me(id: meId, name: 'Ada', defaultCurrency: 'EUR');

  setUp(() async {
    final db = await databaseFactoryMemory.openDatabase(
      'test-${DateTime.now().microsecondsSinceEpoch}',
    );
    await Stores.meta.record('profile').put(db, me.toJson());
    ledger = LocalLedger(() async => db, clock: () => DateTime.utc(2026, 5));
  });

  Matcher validation(String field) =>
      isA<ApiError>().having((e) => e.field, 'field', field);

  test('a malformed custom id is rejected like the server', () async {
    final categories = LocalCategoriesRepository(ledger);
    for (final id in ['climbing', 'custom.1', 'general']) {
      await expectLater(
        categories.create(CategoryInput(id: id, name: 'Climbing')),
        throwsA(
          isA<ApiError>()
              .having((e) => e.field, 'field', 'id')
              .having(
                (e) => e.message,
                'message',
                'Custom category ids look like custom.<uuid>.',
              ),
        ),
        reason: id,
      );
    }
    final made = await categories.create(
      const CategoryInput(
        id: ' CUSTOM.11111111-1111-4111-8111-11111111111A ',
        name: 'Climbing',
      ),
    );
    expect(made.id, 'custom.11111111-1111-4111-8111-11111111111a');
  });

  test('blank icon uses the default', () async {
    final categories = LocalCategoriesRepository(ledger);
    final made = await categories.create(
      const CategoryInput(name: 'Climbing', icon: '', group: ' '),
    );
    expect(made.icon, 'label');
    expect(made.group, 'Other');
    final edited = await categories.update(
      made.id,
      const CategoryInput(icon: 'fitness_center'),
    );
    expect(
      (await categories.update(edited.id, const CategoryInput(icon: ''))).icon,
      'label',
    );
  });

  test('default split: 60/40 rent, stale after a join, cleared', () async {
    final groups = LocalGroupsRepository(ledger);
    var flat = await groups.create(
      const GroupInput(
        name: 'Flat',
        members: [MemberInput(name: 'Ben')],
      ),
    );
    final ben = flat.members[1].id;
    expect(flat.defaultSplit, isNull);

    flat = await groups.update(
      flat.id,
      GroupInput(
        defaultSplit: DefaultSplit(
          method: SplitMethod.percent,
          values: {meId: '60', ben: ' 40 '},
        ),
      ),
    );
    expect(flat.defaultSplit?.method, SplitMethod.percent);
    expect(flat.defaultSplit?.values, {meId: '60', ben: '40'});

    await expectLater(
      groups.update(
        flat.id,
        GroupInput(
          defaultSplit: DefaultSplit(
            method: SplitMethod.percent,
            values: {meId: '60', ben: '60'},
          ),
        ),
      ),
      throwsA(validation('defaultSplit')),
    );

    flat = await groups.addMember(flat.id, name: 'Cy');
    expect(flat.defaultSplit, isNull, reason: 'members changed');
    final cy = flat.members.last.id;
    await groups.removeMember(flat.id, cy);
    expect((await groups.get(flat.id)).defaultSplit?.values[ben], '40');

    flat = await groups.update(
      flat.id,
      const GroupInput(clearDefaultSplit: true),
    );
    expect(flat.defaultSplit, isNull);
  });

  test('custom categories: create, use, delete, keep on edit', () async {
    final categories = LocalCategoriesRepository(ledger);
    final expenses = LocalExpensesRepository(ledger);
    final climbing = await categories.create(
      const CategoryInput(
        name: '  Climbing   gym ',
        icon: 'fitness_center',
        group: 'Entertainment',
      ),
    );
    expect(climbing.id, startsWith('custom.'));
    expect(climbing.name, 'Climbing gym');
    expect(climbing.mine, isTrue);
    expect((await categories.list()).map((c) => c.id), contains(climbing.id));

    await expectLater(
      categories.create(const CategoryInput(name: '')),
      throwsA(validation('name')),
    );
    await expectLater(
      categories.create(const CategoryInput(name: 'X', icon: 'rocket')),
      throwsA(validation('icon')),
    );

    ExpenseInput shoes(String? category) => ExpenseInput(
      description: 'Shoes',
      currency: 'EUR',
      total: 9000,
      date: '2026-04-20',
      category: category,
      payers: const [Payer(userId: meId, amount: 9000)],
      splits: const [SplitEntry(userId: meId)],
    );
    final saved = await expenses.create(shoes(climbing.id));
    expect(saved.category, climbing.id);

    final renamed = await categories.update(
      climbing.id,
      const CategoryInput(name: 'Bouldering'),
    );
    expect(renamed.icon, 'fitness_center');

    await categories.delete(climbing.id);
    expect(
      (await categories.list()).map((c) => c.id),
      isNot(contains(climbing.id)),
    );
    // History is kept and the expense can still be edited...
    final edited = await expenses.update(saved.id, shoes(climbing.id));
    expect(edited.category, climbing.id);
    // ...but new expenses cannot use a deleted category.
    await expectLater(
      expenses.create(shoes(climbing.id)),
      throwsA(validation('category')),
    );

    // The import carries every custom category, deleted ones flagged.
    final body = buildImportRequest(
      meId: meId,
      people: const [],
      groups: const [],
      expenses: await ledger.expenses(),
      categories: await ledger.customCategories(),
    );
    expect(body['categories'], [
      {
        'id': climbing.id,
        'name': 'Bouldering',
        'icon': 'fitness_center',
        'group': 'Entertainment',
        'deleted': true,
      },
    ]);
  });
}
