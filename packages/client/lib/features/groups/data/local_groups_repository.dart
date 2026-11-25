import 'dart:convert';
import 'dart:typed_data';

import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/config/local_config.dart';
import 'package:splitspends/core/money/money.dart';
import 'package:splitspends_ledger/ledger.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';
import 'package:splitspends/features/groups/domain/stats.dart';

/// On-device groups. Same rules as the server: only settled members can be
/// removed and only settled groups deleted.
class LocalGroupsRepository implements GroupsRepository {
  LocalGroupsRepository(this._ledger);

  final LocalLedger _ledger;

  /// The stored default split of a group record when it still matches the
  /// members (the server's rule), else null.
  static JsonMap? effectiveSplit(JsonMap group) {
    final raw = group['defaultSplit'];
    if (raw is! Map) return null;
    final values = ((raw['values'] as Map?) ?? const {}).map(
      (k, v) => MapEntry(k as String, '$v'),
    );
    final method = raw['method'] as String? ?? '';
    if (!defaultSplitMatches(method, values, LocalLedger.memberIds(group))) {
      return null;
    }
    return {'method': method, 'values': values};
  }

  /// Validates [split] against [members] like the server does.
  static JsonMap _checkedSplit(DefaultSplit split, List<String> members) {
    try {
      return {
        'method': split.method.wire,
        'values': validateDefaultSplit(
          split.method.wire,
          split.values,
          members,
        ),
      };
    } on LedgerError catch (error) {
      throw ApiError.validation(error.field, error.message);
    }
  }

  /// The API view of group record [g]; [expenses] may hold other groups'
  /// expenses too.
  static Group _view(
    JsonMap g, {
    required Me me,
    required Map<String, JsonMap> people,
    required List<JsonMap> expenses,
  }) {
    final entries = LocalLedger.groupEntries(expenses, g['id'] as String);
    return Group.fromJson({
      ...g,
      'defaultSplit': effectiveSplit(g),
      'members': [
        for (final id in LocalLedger.memberIds(g))
          people[id] ?? {'id': id, 'name': 'Someone'},
      ],
      'balances': LocalLedger.balancesJson(nets(entries)[me.id] ?? {}),
    });
  }

  @override
  Future<List<Group>> list({bool archived = false}) async {
    final db = await _ledger.db;
    final me = await _ledger.me(db);
    final groups =
        [
          for (final g in await _ledger.groups(db))
            if (LocalLedger.memberIds(g).contains(me.id) &&
                (g['archived'] == true) == archived)
              g,
        ]..sort(
          (a, b) =>
              (b['updatedAt'] as String).compareTo(a['updatedAt'] as String),
        );
    // One read of each store for the whole list: refreshes run this after
    // every change, and the expenses store grows without bound.
    final people = await _ledger.directory(db);
    final byGroup = <Object?, List<JsonMap>>{};
    for (final e in await _ledger.expenses(db)) {
      (byGroup[e['groupId']] ??= []).add(e);
    }
    return [
      for (final g in groups)
        _view(
          g,
          me: me,
          people: people,
          expenses: byGroup[g['id']] ?? const [],
        ),
    ];
  }

  @override
  Future<Group> get(String id) async {
    final db = await _ledger.db;
    return _view(
      await _ledger.group(id, db),
      me: await _ledger.me(db),
      people: await _ledger.directory(db),
      expenses: await _ledger.expenses(db),
    );
  }

  @override
  Future<Group> create(GroupInput input) async {
    final name = input.name?.trim() ?? '';
    if (name.isEmpty) {
      throw const ApiError.validation('name', 'Give the group a name.');
    }
    final db = await _ledger.db;
    final id = input.id ?? _ledger.newId();
    await db.transaction((txn) async {
      final me = await _ledger.me(txn);
      final members = <String>[me.id];
      for (final m in input.members ?? const <MemberInput>[]) {
        final pid = await _ledger.ensurePerson(
          txn,
          email: m.email,
          name: m.name,
        );
        if (!members.contains(pid)) members.add(pid);
      }
      final at = _ledger.stamp();
      await Stores.groups.record(id).put(txn, {
        'id': id,
        'name': name,
        'kind': input.kind?.name ?? 'other',
        'defaultCurrency': input.defaultCurrency ?? me.defaultCurrency,
        'simplifyDebts': input.simplifyDebts ?? true,
        'emoji': input.emoji ?? (input.kind ?? GroupKind.other).emoji,
        'color': input.color ?? '#00897B',
        'archived': false,
        'memberIds': members,
        if (input.defaultSplit != null)
          'defaultSplit': _checkedSplit(input.defaultSplit!, members),
        'createdAt': at,
        'updatedAt': at,
      });
      await _ledger.record(
        txn,
        'group.created',
        groupId: id,
        data: {'groupName': name},
        involved: members,
      );
    });
    return get(id);
  }

  @override
  Future<Group> update(String id, GroupInput input) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final g = await _ledger.group(id, txn);
      final name = input.name?.trim();
      if (name != null && name.isEmpty) {
        throw const ApiError.validation('name', 'Give the group a name.');
      }
      final next = {
        ...g,
        'name': ?name,
        'kind': ?input.kind?.name,
        'defaultCurrency': ?input.defaultCurrency,
        'simplifyDebts': ?input.simplifyDebts,
        'emoji': ?input.emoji,
        'color': ?input.color,
        'archived': ?input.archived,
        if (input.clearDefaultSplit)
          'defaultSplit': null
        else if (input.defaultSplit != null)
          'defaultSplit': _checkedSplit(
            input.defaultSplit!,
            LocalLedger.memberIds(g),
          ),
        'updatedAt': _ledger.stamp(),
      };
      await Stores.groups.record(id).put(txn, next);
      await _ledger.record(
        txn,
        'group.updated',
        groupId: id,
        data: {'groupName': next['name']},
      );
    });
    return get(id);
  }

  @override
  Future<void> delete(String id) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      await _ledger.group(id, txn);
      final entries = LocalLedger.groupEntries(await _ledger.expenses(txn), id);
      if (!nets(entries).values.every((b) => b.settled)) {
        throw const ApiError.conflict('Settle up before deleting this group.');
      }
      await Stores.expenses.delete(
        txn,
        finder: Finder(filter: Filter.equals('groupId', id)),
      );
      await Stores.activity.delete(
        txn,
        finder: Finder(filter: Filter.equals('groupId', id)),
      );
      await Stores.groups.record(id).delete(txn);
    });
  }

  @override
  Future<Group> addMember(String id, {String? email, String? name}) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final g = await _ledger.group(id, txn);
      final pid = await _ledger.ensurePerson(txn, email: email, name: name);
      final members = LocalLedger.memberIds(g);
      final person = (await _ledger.directory(txn))[pid];
      if (members.contains(pid)) {
        throw ApiError.conflict(
          '${person?['name'] ?? 'This person'} is already in this group.',
        );
      }
      await Stores.groups.record(id).put(txn, {
        ...g,
        'memberIds': [...members, pid],
        'updatedAt': _ledger.stamp(),
      });
      await _ledger.record(
        txn,
        'group.member-added',
        groupId: id,
        data: {
          'groupName': g['name'],
          'targetName': person?['name'],
          'targetId': pid,
        },
        involved: [pid],
      );
    });
    return get(id);
  }

  @override
  Future<void> removeMember(String id, String userId) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final g = await _ledger.group(id, txn);
      if (!LocalLedger.memberIds(g).contains(userId)) {
        throw ApiError.notFound('Member');
      }
      // The device owner is the only one who can open the group here; the
      // server refuses to remove the last such member for the same reason.
      if (userId == (await _ledger.me(txn)).id) {
        throw const ApiError.conflict(
          'On this device you cannot leave a group. Archive it to put it '
          'away, or delete it once everyone is settled up.',
        );
      }
      final entries = LocalLedger.groupEntries(await _ledger.expenses(txn), id);
      if (!(nets(entries)[userId]?.settled ?? true)) {
        throw const ApiError.conflict(
          'They need to settle up in this group first.',
        );
      }
      final person = (await _ledger.directory(txn))[userId];
      await Stores.groups.record(id).put(txn, {
        ...g,
        'memberIds': [
          for (final m in LocalLedger.memberIds(g))
            if (m != userId) m,
        ],
        'updatedAt': _ledger.stamp(),
      });
      await _ledger.record(
        txn,
        'group.member-removed',
        groupId: id,
        data: {
          'groupName': g['name'],
          'targetName': person?['name'],
          'targetId': userId,
        },
        involved: [userId],
      );
    });
  }

  @override
  Future<GroupBalances> balances(String id) async {
    final db = await _ledger.db;
    final g = await _ledger.group(id, db);
    final entries = LocalLedger.groupEntries(await _ledger.expenses(db), id);
    final n = nets(entries);
    return GroupBalances.fromJson({
      'members': [
        for (final m in LocalLedger.memberIds(g))
          {'userId': m, 'balances': LocalLedger.balancesJson(n[m] ?? {})},
      ],
      'debts': [
        for (final d in debts(entries, simplified: g['simplifyDebts'] == true))
          {
            'from': d.from,
            'to': d.to,
            'currency': d.currency,
            'amount': d.amount,
          },
      ],
    });
  }

  /// The server's CSV (GET /groups/{id}/export): oldest first, amounts in
  /// the currency's decimals, one column per member and then one for each
  /// former member who still appears in an expense.
  @override
  Future<Uint8List> export(String id) async {
    final db = await _ledger.db;
    final g = await _ledger.group(id, db);
    final people = await _ledger.directory(db);
    String name(String uid) => (people[uid]?['name'] as String?) ?? 'Someone';
    String cell(Object? v) {
      final s = '${v ?? ''}';
      // Quoted exactly when Go's encoding/csv quotes.
      return s.contains(RegExp(r'[",\r\n]')) ||
              s.startsWith(RegExp(r'\s')) ||
              s == r'\.'
          ? '"${s.replaceAll('"', '""')}"'
          : s;
    }

    final list = [
      for (final e in await _ledger.expenses(db))
        if (e['groupId'] == id && !LocalLedger.isDeleted(e)) e,
    ]..sort(_oldestFirst);
    final members = LocalLedger.memberIds(g);
    final former =
        <String>{
          for (final e in list)
            for (final s in (e['shares'] as List).cast<JsonMap>())
              if (!members.contains(s['userId'])) s['userId'] as String,
        }.toList()..sort((a, b) {
          final byName = name(a).compareTo(name(b));
          return byName != 0 ? byName : a.compareTo(b);
        });
    final columns = [...members, ...former];

    final rows = <List<Object?>>[
      [
        'Date',
        'Description',
        'Category',
        'Kind',
        'Currency',
        'Cost',
        for (final c in columns) _safeCell(name(c)),
      ],
    ];
    for (final e in list) {
      final exponent = localConfig.currency(e['currency'] as String).exponent;
      final net = <Object?, int>{};
      for (final s in (e['shares'] as List).cast<JsonMap>()) {
        net[s['userId']] =
            (net[s['userId']] ?? 0) + (s['paid'] as int) - (s['owed'] as int);
      }
      rows.add([
        e['date'],
        _safeCell((e['description'] as String?) ?? ''),
        _safeCell((e['category'] as String?) ?? ''),
        e['kind'],
        e['currency'],
        Money.toPlain(e['total'] as int, exponent),
        for (final c in columns) Money.toPlain(net[c] ?? 0, exponent),
      ]);
    }
    final csv = rows.map((r) => r.map(cell).join(',')).join('\n');
    return Uint8List.fromList(utf8.encode('$csv\n'));
  }

  /// The server's export order: date, then creation, then id.
  static int _oldestFirst(JsonMap a, JsonMap b) {
    for (final key in const ['date', 'createdAt', 'id']) {
      final order = ('${a[key] ?? ''}').compareTo('${b[key] ?? ''}');
      if (order != 0) return order;
    }
    return 0;
  }

  @override
  Future<Stats> stats({String? groupId, String? currency}) async {
    final db = await _ledger.db;
    final me = await _ledger.me(db);
    var code = currency;
    if (groupId != null) {
      final g = await _ledger.group(groupId, db);
      code ??= g['defaultCurrency'] as String;
    }
    code ??= me.defaultCurrency;
    // Same window as the server: the last year up to today.
    final to = _ledger.today();
    final from = DateTime.utc(to.year - 1, to.month, to.day);
    bool inWindow(String day) {
      final d = parseDay(day);
      return d != null && !d.isBefore(from) && !d.isAfter(to);
    }

    final liveCustom = {
      for (final c in await _ledger.customCategories(db))
        if (c['deleted'] != true) c['id'] as String,
    };
    final byCategory = <String, int>{};
    final byMonth = <String, int>{};
    final byMember = <String, ({int paid, int owed})>{};
    for (final e in await _ledger.expenses(db)) {
      if (LocalLedger.isDeleted(e) ||
          e['kind'] != 'expense' ||
          e['currency'] != code ||
          !inWindow(e['date'] as String)) {
        continue;
      }
      final shares = (e['shares'] as List).cast<JsonMap>();
      int amount;
      if (groupId != null) {
        if (e['groupId'] != groupId) continue;
        amount = e['total'] as int;
        for (final s in shares) {
          final m = byMember[s['userId']] ?? (paid: 0, owed: 0);
          byMember[s['userId'] as String] = (
            paid: m.paid + (s['paid'] as int),
            owed: m.owed + (s['owed'] as int),
          );
        }
      } else {
        final mine = shares.where((s) => s['userId'] == me.id).firstOrNull;
        if (mine == null || (mine['owed'] as int) <= 0) continue;
        amount = mine['owed'] as int;
      }
      var cat = e['category'] as String;
      // Like the server: deleted custom categories count as General.
      if (cat.startsWith('custom.') && !liveCustom.contains(cat)) {
        cat = 'general';
      }
      byCategory[cat] = (byCategory[cat] ?? 0) + amount;
      final month = (e['date'] as String).substring(0, 7);
      byMonth[month] = (byMonth[month] ?? 0) + amount;
    }
    final categories = byCategory.entries.toList()
      ..sort((a, b) => b.value.compareTo(a.value));
    final months = byMonth.entries.toList()
      ..sort((a, b) => a.key.compareTo(b.key));
    final members = byMember.entries.toList()
      ..sort((a, b) => b.value.owed.compareTo(a.value.owed));
    return Stats.fromJson({
      'currency': code,
      'totalSpent': byCategory.values.fold<int>(0, (a, b) => a + b),
      'byCategory': [
        for (final c in categories) {'category': c.key, 'amount': c.value},
      ],
      'byMonth': [
        for (final m in months) {'month': m.key, 'amount': m.value},
      ],
      'byMember': [
        for (final m in members)
          {'userId': m.key, 'paid': m.value.paid, 'owed': m.value.owed},
      ],
    });
  }
}

/// Neutralises spreadsheet formula injection in text a person typed, like
/// safeCell in apps/api/routes/expenses.go. Amount cells never pass through
/// it, since a negative amount must stay a number.
String _safeCell(String s) =>
    s.isNotEmpty && '=+-@\t\r'.contains(s[0]) ? "'$s" : s;
