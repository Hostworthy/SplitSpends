import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/core/paging/paged.dart';

/// A list on the server ([rows], newest first), two per page by offset;
/// fails while [failing] is set.
class _Numbers extends PagedNotifier<int> {
  static var rows = <int>[];
  static var failing = false;
  static var fetches = 0;

  @override
  int get pageSize => 2;

  @override
  Object key(int item) => item;

  @override
  Future<PageResult<int>> fetch(int offset, int limit) async {
    fetches++;
    if (failing) throw const ApiError.network('Cannot reach the server.');
    return PageResult(
      items: rows.skip(offset).take(limit).toList(),
      total: rows.length,
    );
  }
}

final _numbers = AsyncNotifierProvider<_Numbers, Paged<int>>(_Numbers.new);

void main() {
  setUp(() {
    _Numbers.rows = [0, 1, 2, 3, 4];
    _Numbers.failing = false;
    _Numbers.fetches = 0;
  });

  test('a page that fails keeps the error until retried', () async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    container.listen(_numbers, (_, _) {});
    await container.read(_numbers.future);
    final feed = container.read(_numbers.notifier);

    _Numbers.failing = true;
    await feed.loadMore();
    var page = container.read(_numbers).requireValue;
    expect(page.error, isA<ApiError>());
    expect(page.items, [0, 1]);
    expect(page.hasMore, isTrue);
    expect(page.loadingMore, isFalse);

    // Scrolling does not try again on its own.
    await feed.loadMore();
    expect(_Numbers.fetches, 2);

    _Numbers.failing = false;
    await feed.retry();
    page = container.read(_numbers).requireValue;
    expect(page.error, isNull);
    expect(page.items, [0, 1, 2, 3]);
    expect(_Numbers.fetches, 3);
  });

  test('rows added while scrolling do not repeat items', () async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    container.listen(_numbers, (_, _) {});
    await container.read(_numbers.future);
    final feed = container.read(_numbers.notifier);

    // Someone adds a row; every older row moves one place down.
    _Numbers.rows = [-1, ..._Numbers.rows];
    await feed.loadMore();
    var page = container.read(_numbers).requireValue;
    expect(page.items, [0, 1, 2]);
    expect(page.hasMore, isTrue);

    await feed.loadMore();
    page = container.read(_numbers).requireValue;
    expect(page.items, [0, 1, 2, 3, 4]);
    expect(page.hasMore, isFalse);
    expect(_Numbers.fetches, 3);

    // Nothing left: scrolling to the end again asks for nothing.
    await feed.loadMore();
    expect(_Numbers.fetches, 3);
  });
}
