import 'package:flutter_test/flutter_test.dart';
import 'package:splitspends/features/activity/domain/activity.dart';
import 'package:splitspends/features/activity/presentation/activity_tile.dart';

Activity _activity(String actorId, String name, String summary) =>
    Activity.fromJson({
      'id': 'a1',
      'kind': 'expense.created',
      'actor': {'id': actorId, 'name': name, 'email': null},
      'summary': summary,
      'data': <String, Object?>{},
      'createdAt': '2026-11-24T12:00:00Z',
    });

void main() {
  test('own actions read as You', () {
    final a = _activity('me', 'Ada', 'Ada added “Dinner” in Lisbon trip');
    expect(personalSummary(a, 'me'), 'You added “Dinner” in Lisbon trip');
  });

  test('other people keep their name', () {
    final a = _activity('ben', 'Ben', 'Ben added “Taxi”');
    expect(personalSummary(a, 'me'), 'Ben added “Taxi”');
  });
}
