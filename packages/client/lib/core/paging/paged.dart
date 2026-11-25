import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/log.dart';
import 'package:splitspends/core/paging/page_result.dart';

/// Accumulated pages of a list endpoint.
class Paged<T> {
  const Paged({
    this.items = const [],
    this.total = 0,
    this.loadingMore = false,
    this.error,
    this._fetched,
  });

  final List<T> items;
  final int total;
  final bool loadingMore;

  /// Rows the server has returned so far, repeats included; the next page
  /// starts here. Rows added on the server while scrolling push older ones
  /// down, so a page can repeat items already shown, which are dropped.
  int get fetched => _fetched ?? items.length;
  final int? _fetched;

  /// Why the last [PagedNotifier.loadMore] failed; scrolling does not load
  /// more until [PagedNotifier.retry].
  final Object? error;

  bool get hasMore => fetched < total;

  Paged<T> copyWith({
    List<T>? items,
    int? total,
    bool? loadingMore,
    int? fetched,
  }) => Paged(
    items: items ?? this.items,
    total: total ?? this.total,
    loadingMore: loadingMore ?? this.loadingMore,
    error: error,
    fetched: fetched ?? this.fetched,
  );
}

/// Base for infinite-scroll feeds: implement [fetch] and [key].
abstract class PagedNotifier<T> extends AsyncNotifier<Paged<T>> {
  int get pageSize => 30;

  Future<PageResult<T>> fetch(int offset, int limit);

  /// What makes an item unique, so a page that repeats one is shown once.
  Object key(T item);

  @override
  Future<Paged<T>> build() async {
    final page = await fetch(0, pageSize);
    return Paged(items: page.items, total: page.total);
  }

  Future<void> loadMore() async {
    final current = state.value;
    if (current == null ||
        !current.hasMore ||
        current.loadingMore ||
        current.error != null) {
      return;
    }
    state = AsyncData(current.copyWith(loadingMore: true));
    try {
      final page = await fetch(current.fetched, pageSize);
      if (!ref.mounted) return;
      final seen = {for (final item in current.items) key(item)};
      state = AsyncData(
        Paged(
          items: [
            ...current.items,
            for (final item in page.items)
              if (seen.add(key(item))) item,
          ],
          total: page.total,
          fetched: current.fetched + page.items.length,
        ),
      );
    } catch (error, stack) {
      log('paging.load.failed', error: error, stack: stack);
      if (!ref.mounted) return;
      state = AsyncData(
        Paged(
          items: current.items,
          total: current.total,
          error: error,
          fetched: current.fetched,
        ),
      );
    }
  }

  /// Loads the page that failed again.
  Future<void> retry() {
    final current = state.value;
    if (current == null || current.error == null) return Future.value();
    state = AsyncData(
      Paged(
        items: current.items,
        total: current.total,
        fetched: current.fetched,
      ),
    );
    return loadMore();
  }

  /// Replaces one item in place (optimistic edits).
  void replaceWhere(bool Function(T) test, T? replacement) {
    final current = state.value;
    if (current == null) return;
    final items = <T>[];
    var removed = 0;
    for (final item in current.items) {
      if (test(item)) {
        if (replacement != null) {
          items.add(replacement);
        } else {
          removed++;
        }
      } else {
        items.add(item);
      }
    }
    state = AsyncData(
      current.copyWith(
        items: items,
        total: current.total - removed,
        fetched: current.fetched - removed,
      ),
    );
  }
}
