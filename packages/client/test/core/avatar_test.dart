import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/theme.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/widgets/avatar.dart';

void main() {
  test('initials take whole characters', () {
    expect(const User(id: 'x', name: 'Ada Lovelace').initials, 'AL');
    expect(const User(id: 'x', name: '🎉 Party').initials, '🎉P');
    expect(const User(id: 'x', name: 'Ünal').initials, 'Ü');
    expect(const User(id: 'x', name: '👩‍👩‍👧 Family').initials, '👩‍👩‍👧F');
    expect(const User(id: 'x', name: '𠀀 Wu').initials, '𠀀W');
  });

  test('Gravatar fields parse and survive copies', () {
    final me = Me.fromJson({
      'id': 'u1',
      'email': 'ada@example.com',
      'name': 'Ada Lovelace',
      'avatarUrl': 'https://gravatar.com/avatar/abc?s=160&d=404',
      'gravatar': true,
    });
    expect(me.gravatar, isTrue);
    expect(me.avatarUrl, contains('gravatar.com'));
    final renamed = me.copyWith(name: 'Ada');
    expect(renamed.gravatar, isTrue);
    expect(renamed.avatarUrl, me.avatarUrl);
    expect(User.fromJson({'id': 'u2', 'name': 'Sam'}).avatarUrl, isNull);
    expect(
      User.fromJson({'id': 'u2', 'name': 'Sam', 'avatarUrl': ''}).avatarUrl,
      isNull,
    );
  });

  testWidgets('initials stay when the picture cannot load', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: UserAvatar(
            user: User(
              id: 'u1',
              name: 'Ada Lovelace',
              avatarUrl: 'https://gravatar.com/avatar/none?s=160&d=404',
            ),
          ),
        ),
      ),
    );
    await tester.pump();
    expect(find.text('AL'), findsOneWidget);
    expect(find.byType(Image), findsOneWidget);
  });
}
