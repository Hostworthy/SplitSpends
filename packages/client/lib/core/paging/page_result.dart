/// One page of a list plus the total count, from any data source.
class PageResult<T> {
  const PageResult({required this.items, required this.total});

  final List<T> items;
  final int total;

  bool get hasMore => items.length < total;
}
