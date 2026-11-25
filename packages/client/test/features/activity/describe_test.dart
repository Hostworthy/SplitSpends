import 'package:flutter_test/flutter_test.dart';
import 'package:splitspends/features/activity/data/local_activity_repository.dart';

Map<String, Object?> _removed(String targetId, String targetName) => {
  'groupName': 'Flat',
  'targetName': targetName,
  'targetId': targetId,
};

void main() {
  test('removing someone with the same name is not leaving', () {
    expect(
      describe(
        'group.member-removed',
        _removed('b2', 'Ben'),
        'Ben',
        actorId: 'b1',
      ),
      'Ben removed Ben from Flat',
    );
  });

  test('leaving is still leaving after a rename', () {
    expect(
      describe(
        'group.member-removed',
        _removed('b1', 'Ben'),
        'Benjamin',
        actorId: 'b1',
      ),
      'Benjamin left Flat',
    );
  });
}
