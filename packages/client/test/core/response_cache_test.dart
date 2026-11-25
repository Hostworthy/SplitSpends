import 'package:flutter_test/flutter_test.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/response_cache.dart';

void main() {
  test('keeps only the most recently written responses', () async {
    final db = await databaseFactoryMemory.openDatabase(
      'cache-${DateTime.now().microsecondsSinceEpoch}',
    );
    var tick = DateTime.utc(2026);
    final cache = SembastResponseCache(
      () async => db,
      clock: () => tick = tick.add(const Duration(seconds: 1)),
    );
    for (var i = 0; i < 400; i++) {
      await cache.write('key-$i', {'n': i});
    }
    expect(await Stores.cache.count(db), 300);
    expect(await cache.read('key-99'), isNull);
    expect(await cache.read('key-100'), {'n': 100});
    expect(await cache.read('key-399'), {'n': 399});
    await db.close();
  });
}
