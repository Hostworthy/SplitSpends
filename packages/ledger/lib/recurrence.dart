// Dart port of expense.Recurrence.Next and the scheduling in
// expense.Service.RunRecurring.

/// The next date of a series. Month and year steps overflow like Go's
/// `AddDate` (Jan 31 + 1 month = Mar 3 in a non-leap year).
DateTime? nextOccurrence(String recurrence, DateTime date) {
  final d = DateTime.utc(date.year, date.month, date.day);
  return switch (recurrence) {
    'weekly' => DateTime.utc(d.year, d.month, d.day + 7),
    'biweekly' => DateTime.utc(d.year, d.month, d.day + 14),
    'monthly' => DateTime.utc(d.year, d.month + 1, d.day),
    'yearly' => DateTime.utc(d.year + 1, d.month, d.day),
    _ => null,
  };
}

/// How many missed occurrences a series books at most when it has fallen
/// behind. Older ones are skipped, so a series that starts long ago cannot
/// flood a group with copies.
const catchUp = 12;

/// The occurrences from [next] up to and including [today] (the latest
/// [catchUp] of them, oldest first) and the first occurrence after today,
/// where the series continues. Port of `expense.Recurrence.Missed`.
(List<DateTime>, DateTime) missed(
  String recurrence,
  DateTime next,
  DateTime today,
) {
  final dates = <DateTime>[];
  var at = next;
  while (!at.isAfter(today)) {
    final following = nextOccurrence(recurrence, at);
    if (following == null) return (const [], next);
    if (dates.length == catchUp) dates.removeAt(0);
    dates.add(at);
    at = following;
  }
  return (dates, at);
}

/// A recurring expense that may be due.
abstract interface class Recurring {
  String get recurrence;
  DateTime? get nextDate;
  bool get deleted;
}

/// What one due run produces: [source] stops recurring, [copyDates] are the
/// new expenses' dates and [copyNext] is the next date of the last copy,
/// which carries the schedule on.
class RecurringStep<T> {
  const RecurringStep(this.source, this.copyDates, this.copyNext);

  final T source;
  final List<DateTime> copyDates;
  final DateTime copyNext;
}

/// Picks due items (oldest first, at most [limit]) like the backend's
/// `Due` query: not deleted, recurring, next date on or before [today].
List<RecurringStep<T>> dueSteps<T extends Recurring>(
  Iterable<T> items,
  DateTime today, {
  int limit = 100,
}) {
  final due =
      items
          .where(
            (e) =>
                !e.deleted &&
                e.recurrence != 'none' &&
                e.nextDate != null &&
                !e.nextDate!.isAfter(today),
          )
          .toList()
        ..sort((a, b) => a.nextDate!.compareTo(b.nextDate!));
  return [
    for (final src in due.take(limit))
      switch (missed(src.recurrence, src.nextDate!, today)) {
        (final dates, final next) => RecurringStep(src, dates, next),
      },
  ];
}
