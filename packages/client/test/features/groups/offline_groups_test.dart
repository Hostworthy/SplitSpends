import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/sync/outbox.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/groups/data/offline_groups_repository.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/domain/groups_repository.dart';

const _ada = (server: 'http://server.test', user: 'ada');

/// A server that answers creates with [failure], or echoes the input.
class _Remote extends Fake implements GroupsRepository {
  final created = <GroupInput>[];
  ApiError? failure;

  @override
  Future<Group> create(GroupInput input) async {
    created.add(input);
    if (failure != null) throw failure!;
    return Group(id: input.id!, name: input.name!);
  }

  @override
  Future<List<Group>> list({bool archived = false}) async => const [];
}

const _rent = GroupInput(
  name: ' Flat ',
  kind: GroupKind.home,
  defaultCurrency: 'EUR',
  simplifyDebts: false,
  emoji: '🏠',
  color: '#00897B',
  members: [MemberInput(name: 'Sam')],
  defaultSplit: DefaultSplit(
    method: SplitMethod.percent,
    values: {'ada': '60', 'sam': '40'},
  ),
);

void main() {
  late Outbox outbox;
  late _Remote remote;
  late OfflineGroupsRepository groups;

  setUp(() async {
    final db = await databaseFactoryMemory.openDatabase(
      'offline-groups-${DateTime.now().microsecondsSinceEpoch}',
    );
    outbox = Outbox(() async => db, owner: _ada);
    remote = _Remote();
    groups = OfflineGroupsRepository(
      remote,
      outbox,
      me: () => const User(id: 'ada', name: 'Ada', defaultCurrency: 'EUR'),
    );
  });

  test('online create sends every field with a client id', () async {
    final flat = await groups.create(_rent);
    final sent = remote.created.single;
    expect(sent.id, flat.id);
    expect(sent.toJson(), {..._rent.toJson(), 'id': flat.id, 'name': 'Flat'});
    expect(await outbox.pending(), isEmpty);
  });

  test('offline create queues with the same id', () async {
    remote.failure = const ApiError.network('offline');
    final flat = await groups.create(_rent);

    final sent = remote.created.single;
    final op = (await outbox.pending()).single;
    expect((op.method, op.path), ('POST', '/groups'));
    expect((op.body! as Map)['id'], flat.id);
    expect(sent.id, flat.id);
    expect(op.body, {..._rent.toJson(), 'id': flat.id, 'name': 'Flat'});
    expect(flat.defaultSplit?.values, {'ada': '60', 'sam': '40'});
    expect((await groups.list()).single.id, flat.id);
  });

  test('a create waits behind writes already queued', () async {
    await outbox.add('DELETE', '/expenses/e1', label: 'Delete “Taxi”');
    final flat = await groups.create(_rent);
    expect(remote.created, isEmpty);
    expect((await outbox.pending()).last.body, containsPair('id', flat.id));
  });

  test('a refusal from the server is not queued', () async {
    remote.failure = const ApiError.validation('name', 'Name is too long.');
    await expectLater(
      groups.create(_rent),
      throwsA(isA<ApiError>().having((e) => e.field, 'field', 'name')),
    );
    expect(await outbox.pending(), isEmpty);
  });
}
