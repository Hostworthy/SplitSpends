import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/invites/domain/invite.dart';
import 'package:splitspends/features/invites/presentation/join_screen.dart';

const _token = 'k3V9xQ2mN8pL4rT7wY1zA6bC0dE5fG8hJ2kM4nP6qR8';

void main() {
  group('JoinLink.parse', () {
    test('reads the server and token from a full link', () {
      final link = JoinLink.parse('https://split.example.com/join/$_token')!;
      expect(link.token, _token);
      expect(link.server, 'https://split.example.com');
    });

    test(
      'keeps ports, sub-paths and tolerates spaces and a missing scheme',
      () {
        final link = JoinLink.parse(
          '  http://192.168.1.20:8080/split/join/$_token/ ',
        )!;
        expect(link.server, 'http://192.168.1.20:8080/split');
        expect(link.token, _token);
        expect(
          JoinLink.parse('split.example.com/join/$_token')!.server,
          'https://split.example.com',
        );
      },
    );

    test('accepts a bare token without a server', () {
      final link = JoinLink.parse(_token)!;
      expect(link.token, _token);
      expect(link.server, isNull);
    });

    test('rejects anything else', () {
      expect(JoinLink.parse(''), isNull);
      expect(JoinLink.parse('hello'), isNull);
      expect(JoinLink.parse('https://split.example.com/groups/abc'), isNull);
      expect(JoinLink.parse('https://split.example.com/join/'), isNull);
      expect(JoinLink.parse('https://split.example.com/join/a b'), isNull);
    });
  });

  group('joinState', () {
    const server = 'https://split.example.com';

    test('signed in to the same server can join', () {
      const session = Session(
        mode: AppMode.connected,
        serverUrl: 'https://split.example.com/',
        token: 't',
      );
      expect(joinState(session, server), JoinState.join);
    });

    test('signed in elsewhere is told to switch', () {
      const session = Session(
        mode: AppMode.connected,
        serverUrl: 'https://other.example.com',
        token: 't',
      );
      expect(joinState(session, server), JoinState.otherServer);
    });

    test('on-device mode explains that invites need a server', () {
      const session = Session(mode: AppMode.local);
      expect(joinState(session, server), JoinState.onDevice);
    });

    test('without a session people sign in first', () {
      expect(joinState(const Session(), server), JoinState.signIn);
      const expired = Session(mode: AppMode.connected, serverUrl: server);
      expect(joinState(expired, server), JoinState.signIn);
    });
  });
}
