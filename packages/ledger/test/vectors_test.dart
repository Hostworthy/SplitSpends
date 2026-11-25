// Runs the shared spec/vectors.json against the Dart ledger, exactly like
// the Go tests in packages/core.
import 'dart:convert';
import 'dart:io';

import 'package:test/test.dart';

import 'package:splitspends_ledger/ledger.dart';

void main() {
  final doc = jsonDecode(
    File('../../spec/vectors.json').readAsStringSync(),
  ) as Map<String, dynamic>;

  final currencies = {
    for (final c in (doc['currencies'] as List).cast<Map<String, dynamic>>())
      c['code'] as String,
  };

  group('split vectors', () {
    for (final c in (doc['splits'] as List).cast<Map<String, dynamic>>()) {
      test(c['name'] as String, () {
        final i = c['input'] as Map<String, dynamic>;
        final request = ExpenseRequest(
          description: (i['description'] as String?) ?? c['name'] as String,
          notes: (i['notes'] as String?) ?? '',
          currency: (i['currency'] as String?) ?? 'USD',
          total: i['total'] as int,
          method: (i['method'] as String?) ?? '',
          recurrence: (i['recurrence'] as String?) ?? '',
          payers: [
            for (final p in (i['payers'] as List? ?? const []))
              PayerInput(p['userId'] as String, p['amount'] as int),
          ],
          splits: [
            for (final s in (i['splits'] as List? ?? const []))
              SplitValueInput(s['userId'] as String, s['value']?.toString()),
          ],
          items: [
            for (final it in (i['items'] as List? ?? const []))
              ItemInput(
                it['name'] as String,
                it['amount'] as int,
                (it['userIds'] as List).cast<String>(),
              ),
          ],
          tax: (i['tax'] as int?) ?? 0,
          tip: (i['tip'] as int?) ?? 0,
          date: i['date'] as String?,
        );
        final today = c['today'] == null
            ? DateTime.utc(2026)
            : parseDay(c['today'] as String)!;
        if (c['error'] != null) {
          expect(
            () => normalize(request, today: today, currencies: currencies),
            throwsA(
              isA<LedgerError>().having((e) => e.field, 'field', c['error']),
            ),
          );
          return;
        }
        final n = normalize(request, today: today, currencies: currencies);
        final got = {for (final s in n.shares) s.userId: s.owed};
        (c['owed'] as Map<String, dynamic>).forEach((id, want) {
          expect(got[id] ?? 0, want, reason: '$id owes');
        });
        if (c['description'] != null) {
          expect(n.description, c['description']);
        }
        if (c['notes'] != null) expect(n.notes, c['notes']);
        if (c['method'] != null) expect(n.method, c['method']);
        if (c['currency'] != null) expect(n.currency, c['currency']);
        final values = {for (final s in n.shares) s.userId: s.value};
        (c['values'] as Map<String, dynamic>? ?? const {}).forEach((id, want) {
          expect(values[id], want, reason: '$id value');
        });
        expect(
          n.shares.map((s) => s.userId).toList(),
          [...n.shares.map((s) => s.userId)]..sort(),
        );
      });
    }
  });

  List<LedgerEntry> entries(List<dynamic> list) => [
    for (final e in list.cast<Map<String, dynamic>>())
      LedgerEntry(
        groupId: e['groupId'] as String?,
        currency: e['currency'] as String,
        shares: [
          for (final s in (e['shares'] as List))
            LedgerShare(
              s['userId'] as String,
              s['paid'] as int,
              s['owed'] as int,
            ),
        ],
      ),
  ];

  group('balance vectors', () {
    for (final b in (doc['balances'] as List).cast<Map<String, dynamic>>()) {
      final list = entries(b['entries'] as List);
      List<LedgerDebt> parse(String key) => [
        for (final d in (b[key] as List))
          LedgerDebt(
            d['from'] as String,
            d['to'] as String,
            d['currency'] as String,
            d['amount'] as int,
          ),
      ];

      group(b['name'] as String, () {
        test('nets', () {
          final got = nets(list);
          (b['nets'] as Map<String, dynamic>).forEach((id, want) {
            (want as Map<String, dynamic>).forEach((cur, amount) {
              expect(got[id]?[cur] ?? 0, amount, reason: '$id $cur');
            });
          });
        });

        test('pairwise', () => expect(pairwise(list), parse('pairwise')));

        test(
          'simplified',
          () => expect(simplify(nets(list)), parse('simplified')),
        );
      });
    }
  });

  group('sheet vectors', () {
    final cases = (doc['sheets'] as List).cast<Map<String, dynamic>>();
    test('exist', () => expect(cases, isNotEmpty));
    for (final c in cases) {
      test(c['name'] as String, () {
        final groups = (c['groups'] as List).cast<Map<String, dynamic>>();
        final sheet = sheetFor(
          c['me'] as String,
          groups: [
            for (final g in groups)
              LedgerGroup(
                g['id'] as String,
                g['name'] as String,
                simplified: g['simplify'] as bool,
              ),
          ],
          groupEntries: {
            for (final g in groups)
              g['id'] as String: entries(g['entries'] as List),
          },
          nonGroup: entries(c['nonGroup'] as List),
        );
        expect(
          {
            for (final e in sheet.friends.entries)
              e.key: {
                'total': _nonZero(e.value.total),
                'byGroup': _byGroup(e.value.byGroup),
                'nonGroup': _nonZero(e.value.nonGroup),
              },
          },
          {
            for (final e in (c['friends'] as Map<String, dynamic>).entries)
              e.key: {
                'total': _nonZero(_balances(e.value['total'])),
                'byGroup': _byGroup({
                  for (final g in (e.value['byGroup'] as Map).entries)
                    g.key as String: _balances(g.value),
                }),
                'nonGroup': _nonZero(_balances(e.value['nonGroup'])),
              },
          },
        );
        expect(_byGroup(sheet.groups), {
          for (final g in (c['groupNets'] as Map).entries)
            g.key as String: _balances(g.value),
        });
        final t = sheet.totals();
        expect(_nonZero(t.net), _nonZero(_balances(c['net'])), reason: 'net');
        expect(
          _nonZero(t.owed),
          _nonZero(_balances(c['owed'])),
          reason: 'owed',
        );
        expect(
          _nonZero(t.owing),
          _nonZero(_balances(c['owing'])),
          reason: 'owing',
        );
      });
    }
  });

  group('recurrence vectors', () {
    final cases = (doc['recurrence'] as List).cast<Map<String, dynamic>>();
    test('exist', () => expect(cases, isNotEmpty));
    for (final c in cases) {
      test(c['name'] as String, () {
        final start = parseDay(c['start'] as String)!;
        var at = start;
        final got = <String>[];
        for (var i = 0; i < (c['dates'] as List).length; i++) {
          at = nextOccurrence(c['recurrence'] as String, at, start.day)!;
          got.add(formatDay(at));
        }
        expect(got, c['dates']);
      });
    }
  });

  group('default split vectors', () {
    final cases = (doc['defaultSplits'] as List).cast<Map<String, dynamic>>();
    test('exist', () => expect(cases, isNotEmpty));
    for (final c in cases) {
      test(c['name'] as String, () {
        final split = c['split'] as Map<String, dynamic>;
        final method = split['method'] as String;
        final values = (split['values'] as Map).cast<String, String>();
        final members = (c['members'] as List).cast<String>();
        if (c['error'] != null) {
          expect(
            () => validateDefaultSplit(method, values, members),
            throwsA(
              isA<LedgerError>().having((e) => e.field, 'field', c['error']),
            ),
          );
          return;
        }
        final saved = validateDefaultSplit(method, values, members);
        if (c['effective'] == null) return;
        expect(
          defaultSplitMatches(
            method,
            saved,
            (c['current'] as List).cast<String>(),
          ),
          c['effective'],
        );
      });
    }
  });
}

Balances _balances(Object? json) => (json as Map).cast<String, int>();

/// Drops settled currencies, which the two ports may or may not keep.
Balances _nonZero(Balances b) => {
  for (final e in b.entries)
    if (e.value != 0) e.key: e.value,
};

Map<String, Balances> _byGroup(Map<String, Balances> groups) => {
  for (final e in groups.entries)
    if (_nonZero(e.value).isNotEmpty) e.key: _nonZero(e.value),
};
