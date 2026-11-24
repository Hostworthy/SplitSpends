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

/// A recurring expense that may be due.
abstract interface class Recurring {
  String get recurrence;
  DateTime? get nextDate;
  bool get deleted;
}

/// What one due run produces: [source] stops recurring, [copyDate] is the
/// new expense's date and [copyNext] its next date.
class RecurringStep<T> {
  const RecurringStep(this.source, this.copyDate, this.copyNext);

  final T source;
  final DateTime copyDate;
  final DateTime? copyNext;
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
      RecurringStep(
        src,
        src.nextDate!,
        nextOccurrence(src.recurrence, src.nextDate!),
      ),
  ];
}
