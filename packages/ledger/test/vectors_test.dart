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

  group('split vectors', () {
    for (final c in (doc['splits'] as List).cast<Map<String, dynamic>>()) {
      test(c['name'] as String, () {
        final i = c['input'] as Map<String, dynamic>;
        final request = ExpenseRequest(
          description: c['name'] as String,
          currency: 'USD',
          total: i['total'] as int,
          method: (i['method'] as String?) ?? '',
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
        );
        if (c['error'] != null) {
          expect(
            () => normalize(request, today: DateTime.utc(2026)),
            throwsA(
              isA<LedgerError>().having((e) => e.field, 'field', c['error']),
            ),
          );
          return;
        }
        final n = normalize(request, today: DateTime.utc(2026));
        final got = {for (final s in n.shares) s.userId: s.owed};
        (c['owed'] as Map<String, dynamic>).forEach((id, want) {
          expect(got[id] ?? 0, want, reason: '$id owes');
        });
        expect(
          n.shares.map((s) => s.userId).toList(),
          [...n.shares.map((s) => s.userId)]..sort(),
        );
      });
    }
  });

  group('balance vectors', () {
    final b = doc['balances'] as Map<String, dynamic>;
    final entries = [
      for (final e in (b['entries'] as List).cast<Map<String, dynamic>>())
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
    List<LedgerDebt> parse(String key) => [
      for (final d in (b[key] as List))
        LedgerDebt(
          d['from'] as String,
          d['to'] as String,
          d['currency'] as String,
          d['amount'] as int,
        ),
    ];

    test('nets', () {
      final got = nets(entries);
      (b['nets'] as Map<String, dynamic>).forEach((id, want) {
        (want as Map<String, dynamic>).forEach((cur, amount) {
          expect(got[id]?[cur] ?? 0, amount, reason: '$id $cur');
        });
      });
    });

    test('pairwise', () => expect(pairwise(entries), parse('pairwise')));

    test(
      'simplified',
      () => expect(simplify(nets(entries)), parse('simplified')),
    );
  });
}
