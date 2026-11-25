import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/utils/dates.dart';

void main() {
  group('latest', () {
    test('east of UTC early in the day, the server is still on yesterday', () {
      // 08:00 on 26 November in Auckland is 19:00 on 25 November UTC.
      expect(
        Dates.latest(DateTime(2026, 11, 26, 8), DateTime.utc(2026, 11, 25, 19)),
        DateTime(2027, 11, 25),
      );
    });

    test('west of UTC late in the day, local today counts', () {
      // 20:00 on 25 November in New York is 01:00 on 26 November UTC.
      expect(
        Dates.latest(DateTime(2026, 11, 25, 20), DateTime.utc(2026, 11, 26, 1)),
        DateTime(2027, 11, 25),
      );
    });

    test('same day in both', () {
      expect(
        Dates.latest(DateTime(2026, 2, 28, 12), DateTime.utc(2026, 2, 28, 12)),
        DateTime(2027, 2, 28),
      );
    });
  });
}
