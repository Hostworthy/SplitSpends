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
  });

  final List<T> items;
  final int total;
  final bool loadingMore;

  /// Why the last [PagedNotifier.loadMore] failed; scrolling does not load
  /// more until [PagedNotifier.retry].
  final Object? error;

  bool get hasMore => items.length < total;

  Paged<T> copyWith({List<T>? items, int? total, bool? loadingMore}) => Paged(
    items: items ?? this.items,
    total: total ?? this.total,
    loadingMore: loadingMore ?? this.loadingMore,
    error: error,
  );
}

/// Base for infinite-scroll feeds: implement [fetch].
abstract class PagedNotifier<T> extends AsyncNotifier<Paged<T>> {
  int get pageSize => 30;

  Future<PageResult<T>> fetch(int offset, int limit);

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
    state = AsyncData(
      Paged(items: current.items, total: current.total, loadingMore: true),
    );
    try {
      final page = await fetch(current.items.length, pageSize);
      if (!ref.mounted) return;
      state = AsyncData(
        Paged(items: [...current.items, ...page.items], total: page.total),
      );
    } catch (error, stack) {
      log('paging.load.failed', error: error, stack: stack);
      if (!ref.mounted) return;
      state = AsyncData(
        Paged(items: current.items, total: current.total, error: error),
      );
    }
  }

  /// Loads the page that failed again.
  Future<void> retry() {
    final current = state.value;
    if (current == null || current.error == null) return Future.value();
    state = AsyncData(Paged(items: current.items, total: current.total));
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
      current.copyWith(items: items, total: current.total - removed),
    );
  }
}
