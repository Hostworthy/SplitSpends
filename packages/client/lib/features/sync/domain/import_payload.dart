import 'package:splitspends_ledger/ledger.dart';

import 'package:splitspends/core/models/json.dart';

/// What will be uploaded, for the confirmation screen.
class ImportSummary {
  const ImportSummary({
    this.groups = 0,
    this.expenses = 0,
    this.people = 0,
    this.categories = 0,
    this.uploaded,
  });

  final int groups;
  final int expenses;
  final int people;

  /// Custom categories that are not deleted.
  final int categories;

  /// Where and when this data was last uploaded, if it was.
  final ({String server, DateTime at})? uploaded;

  bool get isEmpty =>
      groups == 0 && expenses == 0 && people == 0 && categories == 0;
}

/// Builds the `ImportRequest` body (docs/API.md, "Offline Clients and
/// Import") from on-device records. Pure function: easy to test.
///
/// Deleted expenses are left out. Expenses keep their original split input
/// so the server recomputes exactly the same shares. Custom categories are
/// all sent, deleted ones flagged, so expenses keep their category ids. A
/// group's default split is sent only while it still matches the members
/// (otherwise it is treated as absent, like on the server).
JsonMap buildImportRequest({
  required String meId,
  required List<JsonMap> people,
  required List<JsonMap> groups,
  required List<JsonMap> expenses,
  List<JsonMap> categories = const [],
}) {
  final live =
      [
        for (final e in expenses)
          if (e['deleted'] != true) e,
      ]..sort((a, b) {
        final byDate = (a['date'] as String).compareTo(b['date'] as String);
        return byDate != 0
            ? byDate
            : (a['createdAt'] as String? ?? '').compareTo(
                b['createdAt'] as String? ?? '',
              );
      });
  return {
    'meId': meId,
    'people': [
      for (final p in people)
        {
          'id': p['id'],
          'name': p['name'],
          if ((p['email'] as String?)?.isNotEmpty ?? false) 'email': p['email'],
        },
    ],
    'groups': [
      for (final g in groups)
        {
          'id': g['id'],
          'name': g['name'],
          'kind': g['kind'] ?? 'other',
          'defaultCurrency': g['defaultCurrency'],
          'simplifyDebts': g['simplifyDebts'] ?? true,
          'emoji': g['emoji'],
          'color': g['color'],
          'archived': g['archived'] ?? false,
          'memberIds': g['memberIds'] ?? const [],
          'defaultSplit': ?_defaultSplit(g),
        },
    ],
    if (categories.isNotEmpty)
      'categories': [
        for (final c in categories)
          {
            'id': c['id'],
            'name': c['name'],
            'icon': c['icon'],
            'group': c['group'],
            if (c['deleted'] == true) 'deleted': true,
          },
      ],
    'expenses': [for (final e in live) _expense(e)],
  };
}

JsonMap? _defaultSplit(JsonMap group) {
  final raw = group['defaultSplit'];
  if (raw is! Map) return null;
  final method = raw['method'] as String? ?? '';
  final values = ((raw['values'] as Map?) ?? const {}).map(
    (k, v) => MapEntry(k as String, '$v'),
  );
  final members = ((group['memberIds'] as List?) ?? const []).cast<String>();
  try {
    return {
      'method': method,
      'values': validateDefaultSplit(method, values, members),
    };
  } on LedgerError {
    return null;
  }
}

JsonMap _expense(JsonMap e) {
  final method = (e['splitMethod'] as String?) ?? 'equal';
  final shares = ((e['shares'] as List?) ?? const []).cast<JsonMap>();
  final payers =
      (e['payers'] as List?)?.cast<JsonMap>() ??
      [
        for (final s in shares)
          if ((s['paid'] as int) > 0)
            {'userId': s['userId'], 'amount': s['paid']},
      ];
  final splits =
      (e['splits'] as List?)?.cast<JsonMap>() ??
      [
        for (final s in shares)
          if ((s['owed'] as int) > 0 || s['value'] != null)
            {'userId': s['userId'], 'value': s['value']},
      ];
  return {
    'id': e['id'],
    'kind': e['kind'] ?? 'expense',
    'groupId': e['groupId'],
    'description': e['description'],
    'currency': e['currency'],
    'total': e['total'],
    'date': e['date'],
    'category': e['category'],
    'notes': e['notes'] ?? '',
    'splitMethod': method,
    'payers': [
      for (final p in payers) {'userId': p['userId'], 'amount': p['amount']},
    ],
    if (method != 'itemized')
      'splits': [
        for (final s in splits)
          {'userId': s['userId'], if (s['value'] != null) 'value': s['value']},
      ],
    if (method == 'itemized') ...{
      'items': e['items'] ?? const [],
      'tax': e['tax'] ?? 0,
      'tip': e['tip'] ?? 0,
    },
    'recurrence': e['recurrence'] ?? 'none',
  };
}

/// Rewrites user ids in on-device records after an import, using the
/// server's `people` map (local id → server id).
({
  List<JsonMap> people,
  List<JsonMap> groups,
  List<JsonMap> expenses,
  List<JsonMap> activity,
})
applyPeopleMap(
  Map<String, String> map, {
  required List<JsonMap> people,
  required List<JsonMap> groups,
  required List<JsonMap> expenses,
  required List<JsonMap> activity,
}) {
  String id(Object? v) => map[v] ?? v as String;
  List<String> ids(Object? v) => [
    for (final x in (v as List? ?? const [])) id(x),
  ];
  List<JsonMap> rows(Object? v, JsonMap Function(JsonMap) f) => [
    for (final r in (v as List? ?? const []).cast<JsonMap>()) f(r),
  ];
  return (
    people: [
      for (final p in people) {...p, 'id': id(p['id'])},
    ],
    groups: [
      for (final g in groups)
        {
          ...g,
          'memberIds': ids(g['memberIds']),
          if (g['defaultSplit'] is Map)
            'defaultSplit': {
              ...(g['defaultSplit'] as Map).cast<String, Object?>(),
              'values': {
                for (final e
                    in (((g['defaultSplit'] as Map)['values'] as Map?) ??
                            const {})
                        .entries)
                  id(e.key): e.value,
              },
            },
        },
    ],
    expenses: [
      for (final e in expenses)
        {
          ...e,
          'createdBy': e['createdBy'] == null ? null : id(e['createdBy']),
          'updatedBy': e['updatedBy'] == null ? null : id(e['updatedBy']),
          'shares': rows(e['shares'], (s) => {...s, 'userId': id(s['userId'])}),
          'payers': rows(e['payers'], (s) => {...s, 'userId': id(s['userId'])}),
          'splits': rows(e['splits'], (s) => {...s, 'userId': id(s['userId'])}),
          'items': rows(
            e['items'],
            (i) => {...i, 'userIds': ids(i['userIds'])},
          ),
        },
    ],
    activity: [
      for (final a in activity)
        {...a, 'actorId': id(a['actorId']), 'involved': ids(a['involved'])},
    ],
  );
}
