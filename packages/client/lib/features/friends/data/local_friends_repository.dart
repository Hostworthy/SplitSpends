import 'package:sembast/sembast.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends_ledger/ledger.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/friends/domain/friend.dart';
import 'package:splitspends/features/friends/domain/friends_repository.dart';

/// On-device people. Balances use the same sheet as the server: per-group
/// debts (honouring each group's simplification) plus non-group pairwise.
class LocalFriendsRepository implements FriendsRepository {
  LocalFriendsRepository(this._ledger);

  final LocalLedger _ledger;

  static Friend _friend(Map<String, JsonMap> people, String id, Sheet sheet) {
    final user = people[id];
    if (user == null) throw ApiError.notFound('Person');
    return Friend.fromJson({
      'user': user,
      'balances': LocalLedger.balancesJson(sheet.friends[id]?.total ?? {}),
    });
  }

  @override
  Future<List<Friend>> list() async {
    final db = await _ledger.db;
    final sheet = await _ledger.sheet(db);
    final people = (await _ledger.people(db)).values.toList()
      ..sort(
        (a, b) => (a['name'] as String).toLowerCase().compareTo(
          (b['name'] as String).toLowerCase(),
        ),
      );
    final directory = await _ledger.directory(db);
    return [
      for (final p in people) _friend(directory, p['id'] as String, sheet),
    ];
  }

  @override
  Future<Friend> add({String? email, String? name}) async {
    final db = await _ledger.db;
    final id = await db.transaction((txn) async {
      final pid = await _ledger.ensurePerson(txn, email: email, name: name);
      final me = await _ledger.me(txn);
      if (pid == me.id) {
        throw const ApiError.validation('email', 'That is you.');
      }
      final person = (await _ledger.directory(txn))[pid];
      await _ledger.record(
        txn,
        'friend.added',
        data: {'targetName': person?['name']},
        involved: [pid],
      );
      return pid;
    });
    return _friend(await _ledger.directory(db), id, await _ledger.sheet(db));
  }

  @override
  Future<Friend> invite(String userId, String email) async {
    final db = await _ledger.db;
    final mail = email.trim().toLowerCase();
    await db.transaction((txn) async {
      final p = await Stores.people.record(userId).get(txn);
      if (p == null) throw ApiError.notFound('Person');
      final taken = await Stores.people.findFirst(
        txn,
        finder: Finder(filter: Filter.equals('email', mail)),
      );
      if (taken != null && taken.key != userId) {
        throw const ApiError.conflict('Someone else already has that email.');
      }
      await Stores.people.record(userId).put(txn, {...p, 'email': mail});
    });
    return _friend(
      await _ledger.directory(db),
      userId,
      await _ledger.sheet(db),
    );
  }

  @override
  Future<FriendDetail> get(String userId) async {
    final db = await _ledger.db;
    final sheet = await _ledger.sheet(db);
    final people = await _ledger.directory(db);
    final user = people[userId];
    if (user == null) throw ApiError.notFound('Person');
    final view = sheet.friends[userId];
    final groups = {for (final g in await _ledger.groups(db)) g['id']: g};
    return FriendDetail.fromJson({
      'user': user,
      'balances': LocalLedger.balancesJson(view?.total ?? {}),
      'groups': [
        for (final e in (view?.byGroup ?? <String, Balances>{}).entries)
          if (!e.value.settled)
            {
              'groupId': e.key,
              'groupName': groups[e.key]?['name'] ?? '',
              'balances': LocalLedger.balancesJson(e.value),
            },
      ],
      'nonGroup': LocalLedger.balancesJson(view?.nonGroup ?? {}),
    });
  }

  @override
  Future<void> remove(String userId) async {
    final db = await _ledger.db;
    await db.transaction((txn) async {
      final sheet = await _ledger.sheet(txn);
      if (!(sheet.friends[userId]?.total.settled ?? true)) {
        throw const ApiError.conflict('Settle up before removing them.');
      }
      final inGroup = (await _ledger.groups(txn))
          .any((g) => LocalLedger.memberIds(g).contains(userId));
      final inExpense = (await _ledger.expenses(txn))
          .any((JsonMap e) => LocalLedger.involves(e, userId));
      if (inGroup || inExpense) {
        throw const ApiError.conflict(
          'They are part of groups or expenses, so they stay in your list.',
        );
      }
      await Stores.people.record(userId).delete(txn);
    });
  }
}
