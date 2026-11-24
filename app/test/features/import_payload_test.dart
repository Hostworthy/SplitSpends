import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/features/sync/domain/import_payload.dart';

void main() {
  const me = 'aaaaaaaa-0000-4000-8000-000000000001';
  const sam = 'aaaaaaaa-0000-4000-8000-000000000002';
  const kim = 'aaaaaaaa-0000-4000-8000-000000000003';
  const group = 'bbbbbbbb-0000-4000-8000-000000000001';

  final people = [
    {'id': sam, 'name': 'Sam', 'email': 'sam@example.com', 'createdAt': 'x'},
    {'id': kim, 'name': 'Kim', 'email': null, 'createdAt': 'x'},
  ];
  final groups = [
    {
      'id': group,
      'name': 'Camping',
      'kind': 'trip',
      'defaultCurrency': 'EUR',
      'simplifyDebts': true,
      'emoji': '⛺',
      'color': '#00897B',
      'archived': false,
      'memberIds': [me, sam, kim],
      'createdAt': 'x',
      'updatedAt': 'x',
    },
  ];
  final expenses = [
    {
      'id': 'e2',
      'groupId': group,
      'kind': 'payment',
      'description': 'Payment',
      'category': 'payment',
      'currency': 'EUR',
      'total': 1000,
      'date': '2026-11-22',
      'notes': '',
      'splitMethod': 'exact',
      'shares': [
        {'userId': sam, 'paid': 1000, 'owed': 0, 'value': null},
        {'userId': me, 'paid': 0, 'owed': 1000, 'value': '1000'},
      ],
      'payers': [
        {'userId': sam, 'amount': 1000},
      ],
      'splits': [
        {'userId': me, 'value': '1000'},
      ],
      'items': <Object>[],
      'tax': 0,
      'tip': 0,
      'recurrence': 'none',
      'deleted': false,
      'createdAt': '2026-11-22T10:00:00Z',
    },
    {
      'id': 'e1',
      'groupId': group,
      'kind': 'expense',
      'description': 'Tent',
      'category': 'general',
      'currency': 'EUR',
      'total': 3000,
      'date': '2026-11-20',
      'notes': 'two nights',
      'splitMethod': 'percent',
      'shares': <Object>[],
      'payers': [
        {'userId': me, 'amount': 3000},
      ],
      'splits': [
        {'userId': me, 'value': '50'},
        {'userId': sam, 'value': '25'},
        {'userId': kim, 'value': '25'},
      ],
      'items': <Object>[],
      'tax': 0,
      'tip': 0,
      'recurrence': 'monthly',
      'deleted': false,
      'createdAt': '2026-11-20T10:00:00Z',
    },
    {
      'id': 'gone',
      'groupId': null,
      'kind': 'expense',
      'description': 'Deleted',
      'currency': 'EUR',
      'total': 1,
      'date': '2026-11-01',
      'splitMethod': 'equal',
      'shares': <Object>[],
      'deleted': true,
    },
  ];

  test('builds the ImportRequest documented in docs/API.md', () {
    final body = buildImportRequest(
      meId: me,
      people: people,
      groups: groups,
      expenses: expenses,
    );
    expect(body['meId'], me);
    expect(body['people'], [
      {'id': sam, 'name': 'Sam', 'email': 'sam@example.com'},
      {'id': kim, 'name': 'Kim'},
    ]);
    final g = (body['groups'] as List).single as Map;
    expect(g['memberIds'], [me, sam, kim]);
    expect(g.containsKey('createdAt'), isFalse);

    final list = (body['expenses'] as List).cast<Map<String, dynamic>>();
    expect(list.map((e) => e['id']), [
      'e1',
      'e2',
    ], reason: 'oldest first, no deleted');
    expect(list[0]['splitMethod'], 'percent');
    expect(list[0]['splits'], [
      {'userId': me, 'value': '50'},
      {'userId': sam, 'value': '25'},
      {'userId': kim, 'value': '25'},
    ]);
    expect(list[0]['recurrence'], 'monthly');
    expect(list[0].containsKey('items'), isFalse);
    expect(list[1]['kind'], 'payment');
    expect(list[1]['payers'], [
      {'userId': sam, 'amount': 1000},
    ]);
    expect(list[1]['splits'], [
      {'userId': me, 'value': '1000'},
    ]);
  });

  test('applies the returned people map everywhere', () {
    const serverSam = 'cccccccc-0000-4000-8000-000000000009';
    final out = applyPeopleMap(
      {sam: serverSam},
      people: people,
      groups: groups,
      expenses: expenses,
      activity: [
        {
          'id': 'a1',
          'actorId': sam,
          'involved': [me, sam],
        },
      ],
    );
    expect(out.people.first['id'], serverSam);
    expect(out.groups.single['memberIds'], [me, serverSam, kim]);
    expect((out.expenses.first['payers'] as List).single['userId'], serverSam);
    expect((out.expenses.first['shares'] as List).first['userId'], serverSam);
    expect(out.activity.single['actorId'], serverSam);
    expect(out.activity.single['involved'], [me, serverSam]);
  });
}
