import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/paging/page_result.dart';
import 'package:splitspends/core/paging/paged.dart';

/// Five numbers, two per page; fails while [failing] is set.
class _Numbers extends PagedNotifier<int> {
  static var failing = false;
  static var fetches = 0;

  @override
  int get pageSize => 2;

  @override
  Future<PageResult<int>> fetch(int offset, int limit) async {
    fetches++;
    if (failing) throw const ApiError.network('Cannot reach the server.');
    final items = [for (var i = offset; i < offset + limit && i < 5; i++) i];
    return PageResult(items: items, total: 5);
  }
}

final _numbers = AsyncNotifierProvider<_Numbers, Paged<int>>(_Numbers.new);

void main() {
  setUp(() {
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
}
