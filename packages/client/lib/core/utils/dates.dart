import 'package:intl/intl.dart';

/// Date helpers for the API's `YYYY-MM-DD` and RFC 3339 strings.
abstract final class Dates {
  static final _api = DateFormat('yyyy-MM-dd');

  static String toApi(DateTime date) => _api.format(date);

  static DateTime parseDay(String value) => DateTime.parse(value);

  static DateTime? tryParse(String? value) =>
      value == null ? null : DateTime.tryParse(value)?.toLocal();

  static DateTime today() {
    final now = DateTime.now();
    return DateTime(now.year, now.month, now.day);
  }

  /// The days a date picker offers: what the server accepts, from 2000 up
  /// to a year from today, widened to include [date] so an expense stored
  /// outside that range (before the rule, or by another client) can still
  /// be opened and moved into it.
  static ({DateTime first, DateTime last}) pickable(DateTime date) {
    final now = today();
    final first = DateTime(2000);
    final last = DateTime(now.year + 1, now.month, now.day);
    return (
      first: date.isBefore(first) ? date : first,
      last: date.isAfter(last) ? date : last,
    );
  }

  /// `Nov 20` this year, `Nov 20, 2025` otherwise.
  static String short(DateTime date) {
    final now = DateTime.now();
    return date.year == now.year
        ? DateFormat.MMMd().format(date)
        : DateFormat.yMMMd().format(date);
  }

  static String monthTitle(DateTime date) => DateFormat.yMMMM().format(date);

  /// `2026-11` → `Nov`.
  static String monthLabel(String yearMonth) {
    final parsed = DateTime.tryParse('$yearMonth-01');
    return parsed == null ? yearMonth : DateFormat.MMM().format(parsed);
  }

  /// `just now`, `5m`, `3h`, `2d`, then a short date.
  static String relative(DateTime time) {
    final diff = DateTime.now().difference(time);
    if (diff.inMinutes < 1) return 'just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    if (diff.inDays < 7) return '${diff.inDays}d ago';
    return short(time);
  }
}
