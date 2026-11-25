import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/domain/stats.dart';
import 'package:splitspends/features/groups/presentation/detail/stats_tab.dart';

/// Records the currency of every stats request.
class _StatsRepo extends Fake implements GroupsRepository {
  final currencies = <String?>[];

  @override
  Future<Stats> stats({String? groupId, String? currency}) async {
    currencies.add(currency);
    return Stats(currency: currency ?? 'USD');
  }
}

Group _group(String currency) =>
    Group(id: 'g1', name: 'Trip', defaultCurrency: currency);

void main() {
  group('fillMonths', () {
    List<(String, int)> pairs(List<MonthAmount> months) => [
      for (final m in months) (m.month, m.amount),
    ];

    test('adds a zero for a month without spending', () {
      final months = fillMonths(const [
        MonthAmount('2026-01', 500),
        MonthAmount('2026-03', 700),
      ]);
      expect(pairs(months), [
        ('2026-01', 500),
        ('2026-02', 0),
        ('2026-03', 700),
      ]);
    });

    test('covers the whole range across a year end', () {
      final months = fillMonths(
        const [MonthAmount('2026-01', 500)],
        first: '2025-11',
        last: '2026-02',
      );
      expect(pairs(months), [
        ('2025-11', 0),
        ('2025-12', 0),
        ('2026-01', 500),
        ('2026-02', 0),
      ]);
    });

    test('a past year spans thirteen calendar months', () {
      expect(
        fillMonths(const [], first: '2025-11', last: '2026-11'),
        hasLength(13),
      );
    });

    test('nothing to fill gives nothing', () {
      expect(fillMonths(const []), isEmpty);
    });
  });

  testWidgets('stats follow a change of the group default currency', (
    tester,
  ) async {
    final repo = _StatsRepo();
    Widget app(Group g) => ProviderScope(
      overrides: [
        groupsRepositoryProvider.overrideWithValue(repo),
        configValueProvider.overrideWithValue(localConfig),
      ],
      child: MaterialApp(
        home: Scaffold(body: GroupStatsTab(group: g)),
      ),
    );
    await tester.pumpWidget(app(_group('EUR')));
    await tester.pumpAndSettle();
    expect(find.text('Only expenses in EUR are counted.'), findsOneWidget);

    await tester.pumpWidget(app(_group('GBP')));
    await tester.pumpAndSettle();
    expect(repo.currencies, ['EUR', 'GBP']);
    expect(find.text('Only expenses in GBP are counted.'), findsOneWidget);
  });
}
