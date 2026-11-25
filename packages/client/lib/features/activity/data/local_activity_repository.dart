import 'package:sembast/sembast.dart';

import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/activity/domain/activity.dart';
import 'package:splitspends/features/activity/domain/activity_repository.dart';

/// On-device activity feed, summarized like the server's `Describe`.
class LocalActivityRepository implements ActivityRepository {
  LocalActivityRepository(this._ledger);

  final LocalLedger _ledger;

  @override
  Future<PageResult<Activity>> list({
    String? groupId,
    String? expenseId,
    int limit = 30,
    int offset = 0,
  }) async {
    final db = await _ledger.db;
    final me = await _ledger.me(db);
    final people = await _ledger.directory(db);
    final rows =
        [
          for (final r in await Stores.activity.find(db))
            if ((groupId == null || r.value['groupId'] == groupId) &&
                (expenseId == null || r.value['expenseId'] == expenseId))
              r.value,
        ]..sort((a, b) {
          final byTime = (b['createdAt'] as String).compareTo(
            a['createdAt'] as String,
          );
          return byTime != 0
              ? byTime
              : ((b['seq'] as int?) ?? 0).compareTo((a['seq'] as int?) ?? 0);
        });
    return PageResult(
      items: [
        for (final a in rows.skip(offset).take(limit))
          Activity.fromJson({
            ...a,
            'actor':
                people[a['actorId']] ?? {'id': a['actorId'], 'name': 'Someone'},
            'summary': describe(
              a['kind'] as String,
              (a['data'] as JsonMap?) ?? const {},
              a['actorId'] == me.id ? 'You' : _name(people[a['actorId']]),
            ),
          }),
      ],
      total: rows.length,
    );
  }

  static String _name(JsonMap? person) =>
      (person?['name'] as String?)?.split(' ').first ?? 'Someone';
}

/// Port of the server's `activity.Describe`.
String describe(String kind, JsonMap data, String actor) {
  String str(String k) => (data[k] as String?) ?? '';
  final group = str('groupName');
  final inGroup = group.isEmpty ? '' : ' in $group';
  return switch (kind) {
    'expense.created' => '$actor added “${str('description')}”$inGroup',
    'expense.updated' => '$actor updated “${str('description')}”$inGroup',
    'expense.deleted' => '$actor deleted “${str('description')}”$inGroup',
    'expense.restored' => '$actor restored “${str('description')}”$inGroup',
    'expense.stopped' =>
      group.isEmpty
          ? '“${str('description')}” stopped repeating because someone in it is no longer connected'
          : '“${str('description')}” stopped repeating$inGroup because someone in it left the group',
    'payment.created' =>
      '$actor recorded a payment from ${str('fromName')} to ${str('toName')}$inGroup',
    'comment.created' => '$actor commented on “${str('description')}”',
    'group.created' => '$actor created the group $group',
    'group.updated' => '$actor updated the group $group',
    'group.member-added' => '$actor added ${str('targetName')} to $group',
    'group.member-removed' =>
      str('targetName') == actor
          ? '$actor left $group'
          : '$actor removed ${str('targetName')} from $group',
    'friend.added' => '$actor added ${str('targetName')} as a friend',
    _ => '$actor did something',
  };
}
