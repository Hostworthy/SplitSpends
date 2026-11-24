import 'package:sembast/sembast.dart';
import 'package:uuid/uuid.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends_ledger/ledger.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';

/// Shared access to the on-device ledger. Records are stored in the same
/// shape the API returns, so feature models parse them unchanged:
///
/// * person: `{ id, name, email?, avatarColor?, createdAt }`
/// * group: `{ id, name, kind, defaultCurrency, simplifyDebts, emoji, color,
///   archived, memberIds, defaultSplit?, createdAt, updatedAt }` (the
///   stored default split; views drop it once it no longer matches)
/// * expense: the API `Expense` minus caller-specific fields, plus `nextDate`
/// * activity: `{ id, kind, actorId, groupId, expenseId, data, involved,
///   createdAt }`
class LocalLedger {
  LocalLedger(this._open, {DateTime Function()? clock})
    : _clock = clock ?? DateTime.now;

  final Future<Database> Function() _open;
  final DateTime Function() _clock;

  static const _uuid = Uuid();
  static int _seq = 0;

  Future<Database> get db => _open();

  String newId() => _uuid.v4();

  DateTime now() => _clock().toUtc();

  String stamp() => now().toIso8601String();

  DateTime today() {
    final n = _clock();
    return DateTime.utc(n.year, n.month, n.day);
  }

  Set<String> get currencies => {
    for (final c in localConfig.currencies) c.code,
  };

  Set<String> get categories => {for (final c in localConfig.categories) c.id};

  /// Custom category records, deleted ones included.
  Future<List<JsonMap>> customCategories([DatabaseClient? client]) async => [
    for (final r in await Stores.categories.find(client ?? await db)) r.value,
  ];

  /// Categories a new expense may use: the built-ins and your live custom
  /// ones, plus [kept] (the category an edited expense already has), like
  /// the server.
  Future<Set<String>> usableCategories(
    DatabaseClient client, {
    String? kept,
  }) async => {
    ...categories,
    for (final c in await customCategories(client))
      if (c['deleted'] != true) c['id'] as String,
    ?kept,
  };

  // ------------------------------------------------------------------ people

  Future<Me> me([DatabaseClient? client]) async {
    final raw = await Stores.meta.record('profile').get(client ?? await db);
    if (raw == null) {
      throw const ApiError.needsServer('Set up on-device mode first.');
    }
    return Me.fromJson(raw as JsonMap);
  }

  Future<Map<String, JsonMap>> people([DatabaseClient? client]) async {
    final records = await Stores.people.find(client ?? await db);
    return {for (final r in records) r.key: r.value};
  }

  /// Everyone by id, including you, as API-shaped user maps.
  Future<Map<String, JsonMap>> directory([DatabaseClient? client]) async {
    final c = client ?? await db;
    final me = await this.me(c);
    final others = await people(c);
    return {
      for (final e in others.entries) e.key: personJson(e.value, me),
      me.id: {...me.toJson(), 'status': 'active'},
    };
  }

  JsonMap personJson(JsonMap person, Me me) => {
    'id': person['id'],
    'name': person['name'] ?? '',
    'email': person['email'],
    'avatarColor': person['avatarColor'],
    'defaultCurrency': me.defaultCurrency,
    'status': 'active',
  };

  /// Finds a person by email (case-insensitive) or creates one. Names are
  /// not unique: a name-only add always creates a new person.
  Future<String> ensurePerson(
    DatabaseClient client, {
    String? email,
    String? name,
  }) async {
    final mail = email?.trim().toLowerCase();
    final label = name?.trim();
    if ((mail == null || mail.isEmpty) && (label == null || label.isEmpty)) {
      throw const ApiError.validation('name', 'Enter a name or an email.');
    }
    final me = await this.me(client);
    if (mail != null && mail.isNotEmpty) {
      if (me.email?.toLowerCase() == mail) return me.id;
      final found = await Stores.people.findFirst(
        client,
        finder: Finder(filter: Filter.equals('email', mail)),
      );
      if (found != null) return found.key;
    }
    final id = newId();
    await Stores.people.record(id).put(client, {
      'id': id,
      'name': (label == null || label.isEmpty) ? mail!.split('@').first : label,
      'email': (mail == null || mail.isEmpty) ? null : mail,
      'createdAt': stamp(),
    });
    return id;
  }

  // ------------------------------------------------------------- groups etc.

  Future<List<JsonMap>> groups([DatabaseClient? client]) async =>
      (await Stores.groups.find(client ?? await db))
          .map((r) => r.value)
          .toList();

  Future<JsonMap> group(String id, [DatabaseClient? client]) async {
    final g = await Stores.groups.record(id).get(client ?? await db);
    if (g == null) throw ApiError.notFound('Group');
    return g;
  }

  Future<List<JsonMap>> expenses([DatabaseClient? client]) async =>
      (await Stores.expenses.find(client ?? await db))
          .map((r) => r.value)
          .toList();

  static List<String> memberIds(JsonMap group) =>
      ((group['memberIds'] as List?) ?? const []).cast<String>();

  static bool isDeleted(JsonMap expense) => expense['deleted'] == true;

  static LedgerEntry entry(JsonMap e) => LedgerEntry(
    groupId: e['groupId'] as String?,
    currency: e['currency'] as String,
    shares: [
      for (final s in (e['shares'] as List).cast<JsonMap>())
        LedgerShare(s['userId'] as String, s['paid'] as int, s['owed'] as int),
    ],
  );

  static bool involves(JsonMap e, String userId) =>
      (e['shares'] as List).cast<JsonMap>().any((s) => s['userId'] == userId);

  /// Live entries of one group.
  static List<LedgerEntry> groupEntries(
    List<JsonMap> expenses,
    String groupId,
  ) => [
    for (final e in expenses)
      if (e['groupId'] == groupId && !isDeleted(e)) entry(e),
  ];

  /// The caller's full balance sheet (friends, groups, totals).
  Future<Sheet> sheet([DatabaseClient? client]) async {
    final c = client ?? await db;
    final me = await this.me(c);
    final all = await expenses(c);
    final mine = [
      for (final g in await groups(c))
        if (memberIds(g).contains(me.id)) g,
    ];
    return sheetFor(
      me.id,
      groups: [
        for (final g in mine)
          LedgerGroup(
            g['id'] as String,
            g['name'] as String,
            simplified: g['simplifyDebts'] == true,
          ),
      ],
      groupEntries: {
        for (final g in mine)
          g['id'] as String: groupEntries(all, g['id'] as String),
      },
      nonGroup: [
        for (final e in all)
          if (e['groupId'] == null && !isDeleted(e) && involves(e, me.id))
            entry(e),
      ],
    );
  }

  static List<JsonMap> balancesJson(Balances b) => [
    for (final e in b.listed())
      Balance(currency: e.key, amount: e.value).toJson(),
  ];

  // ---------------------------------------------------------------- activity

  Future<void> record(
    DatabaseClient client,
    String kind, {
    String? groupId,
    String? expenseId,
    Map<String, Object?> data = const {},
    Iterable<String> involved = const [],
  }) async {
    final me = await this.me(client);
    final id = newId();
    await Stores.activity.record(id).put(client, {
      'id': id,
      'kind': kind,
      'actorId': me.id,
      'groupId': groupId,
      'expenseId': expenseId,
      'data': data,
      'involved': {me.id, ...involved}.toList(),
      'createdAt': stamp(),
      // Orders events recorded within the same instant.
      'seq': DateTime.now().microsecondsSinceEpoch * 1000 + (_seq++ % 1000),
    });
  }

  // --------------------------------------------------------------- recurring

  /// Creates due copies of recurring expenses, like the server's
  /// RunRecurring: the schedule moves to the newest copy. Returns how many
  /// copies were made.
  Future<int> runRecurring() async {
    final database = await db;
    var created = 0;
    // Each pass mirrors one server run; repeat until nothing is due so a
    // device that was off for months catches up.
    for (var pass = 0; pass < 120; pass++) {
      final items = [
        for (final e in await expenses(database)) _RecurringRecord(e),
      ];
      final steps = dueSteps(items, today());
      if (steps.isEmpty) break;
      await database.transaction((txn) async {
        for (final step in steps) {
          final src = step.source.raw;
          final at = stamp();
          final copy = {
            ...src,
            'id': newId(),
            'date': formatDay(step.copyDate),
            'nextDate': step.copyNext == null
                ? null
                : formatDay(step.copyNext!),
            'createdAt': at,
            'updatedAt': at,
            'receiptId': null,
          };
          await Stores.expenses.record(src['id'] as String).put(txn, {
            ...src,
            'recurrence': 'none',
            'nextDate': null,
            'updatedAt': at,
          });
          await Stores.expenses.record(copy['id'] as String).put(txn, copy);
          await record(
            txn,
            'expense.created',
            groupId: copy['groupId'] as String?,
            expenseId: copy['id'] as String,
            data: {
              'description': copy['description'],
              'amount': copy['total'],
              'currency': copy['currency'],
              'recurring': true,
            },
          );
          created++;
        }
      });
    }
    return created;
  }
}

class _RecurringRecord implements Recurring {
  _RecurringRecord(this.raw);

  final JsonMap raw;

  @override
  String get recurrence => (raw['recurrence'] as String?) ?? 'none';

  @override
  DateTime? get nextDate {
    final v = raw['nextDate'] as String?;
    return v == null ? null : parseDay(v);
  }

  @override
  bool get deleted => LocalLedger.isDeleted(raw);
}
