import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/ledger.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/friends/data/local_friends_repository.dart';
import 'package:splitspends/features/friends/data/providers.dart';
import 'package:splitspends/features/friends/presentation/providers.dart';
import 'package:splitspends/features/groups/data/local_groups_repository.dart';
import 'package:splitspends/features/groups/data/providers.dart';
import 'package:splitspends/features/groups/domain/group.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';

void main() {
  const me = Me(
    id: '00000000-0000-4000-8000-00000000000a',
    name: 'Ada',
    defaultCurrency: 'EUR',
  );
  late LocalLedger ledger;
  late ProviderContainer container;

  setUp(() async {
    final db = await databaseFactoryMemory.openDatabase(
      'test-${DateTime.now().microsecondsSinceEpoch}',
    );
    await Stores.meta.record('profile').put(db, me.toJson());
    ledger = LocalLedger(() async => db);
    container = ProviderContainer(
      overrides: [
        groupsRepositoryProvider.overrideWithValue(
          LocalGroupsRepository(ledger),
        ),
        friendsRepositoryProvider.overrideWithValue(
          LocalFriendsRepository(ledger),
        ),
      ],
    );
    addTearDown(container.dispose);
  });

  test('a provider that watches the ledger rebuilds on a change', () {
    var builds = 0;
    final watcher = Provider<int>((ref) {
      ref.watch(ledgerVersionProvider);
      return ++builds;
    });
    final subscription = container.listen(watcher, (_, _) {});
    addTearDown(subscription.close);

    expect(container.read(watcher), 1);
    refreshLedgerIn(container);
    expect(container.read(watcher), 2);
  });

  test('a ledger change refetches the lists that show it', () async {
    final groups = container.listen(groupsProvider, (_, _) {});
    final friends = container.listen(friendsProvider, (_, _) {});
    addTearDown(groups.close);
    addTearDown(friends.close);
    expect(await container.read(groupsProvider.future), isEmpty);
    expect(await container.read(friendsProvider.future), isEmpty);

    await LocalGroupsRepository(ledger).create(
      const GroupInput(
        name: 'Flat',
        members: [MemberInput(name: 'Sam')],
      ),
    );
    refreshLedgerIn(container);

    final listed = await container.read(groupsProvider.future);
    expect(listed.map((g) => g.name), ['Flat']);
    final people = await container.read(friendsProvider.future);
    expect(people.map((f) => f.user.name), ['Sam']);
  });
}
