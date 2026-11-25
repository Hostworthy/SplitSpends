import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/api/api_error.dart';

/// Answers with headers at once, then a body that [body] produces.
class _Client extends http.BaseClient {
  _Client(this.body);

  final Stream<List<int>> Function() body;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    await request.finalize().drain<void>();
    return http.StreamedResponse(body(), 200);
  }
}

/// Offline copies kept in memory.
class _Cache implements ResponseCache {
  final entries = <String, Object?>{};

  @override
  Future<Object?> read(String key) async => entries[key];

  @override
  Future<void> write(String key, Object? body) async => entries[key] = body;

  @override
  Future<void> clear() async => entries.clear();
}

/// A body that starts and then loses its connection.
Stream<List<int>> _dropped() async* {
  yield utf8.encode('{"data": {"id": ');
  throw http.ClientException('Connection reset by peer');
}

/// A body that starts and then never finishes.
Stream<List<int>> _stalled() async* {
  yield utf8.encode('{"data": ');
  await Completer<void>().future;
}

void main() {
  final reachable = <bool>[];

  setUp(reachable.clear);

  ApiClient client(
    Stream<List<int>> Function() body, {
    Duration timeout = const Duration(seconds: 30),
  }) => ApiClient(
    serverUrl: 'http://server.test',
    token: 't',
    httpClient: _Client(body),
    onReachability: reachable.add,
    timeout: timeout,
  );

  Matcher network() =>
      throwsA(isA<ApiError>().having((e) => e.isNetwork, 'isNetwork', isTrue));

  group('a response body that fails after the headers', () {
    test('is a network error, and the server counts as unreachable', () async {
      await expectLater(client(_dropped).getData('/me', (d) => d), network());
      expect(reachable, [false]);
    });

    test('is a network error on uploads too', () async {
      await expectLater(
        client(_dropped).upload(
          '/receipts',
          field: 'file',
          bytes: Uint8List(1),
          filename: 'r.jpg',
          contentType: 'image/jpeg',
          parse: (d) => d,
        ),
        network(),
      );
    });

    test('times out when it never finishes', () async {
      await expectLater(
        client(
          _stalled,
          timeout: const Duration(milliseconds: 10),
        ).getData('/me', (d) => d),
        throwsA(
          isA<ApiError>().having(
            (e) => e.message,
            'message',
            'The server took too long to respond.',
          ),
        ),
      );
      expect(reachable, [false]);
    });
  });

  group('answers', () {
    var unauthorized = 0;
    late _Cache cache;

    setUp(() {
      unauthorized = 0;
      cache = _Cache();
    });

    ApiClient answering(Future<http.Response> Function(http.Request) handler) =>
        ApiClient(
          serverUrl: 'http://server.test',
          token: 't',
          httpClient: MockClient(handler),
          onReachability: reachable.add,
          onUnauthorized: () => unauthorized++,
          cache: cache,
        );

    ApiClient offline() =>
        answering((_) => throw http.ClientException('Network is unreachable'));

    test('reads the error envelope', () async {
      final api = answering(
        (_) async => http.Response(
          jsonEncode({
            'error': 'Enter a name.',
            'code': 'VALIDATION_ERROR',
            'field': 'name',
            'requestId': 'r1',
          }),
          422,
        ),
      );
      await expectLater(
        api.postData('/groups', {'name': ''}, (d) => d),
        throwsA(
          isA<ApiError>()
              .having((e) => e.status, 'status', 422)
              .having((e) => e.message, 'message', 'Enter a name.')
              .having((e) => e.code, 'code', 'VALIDATION_ERROR')
              .having((e) => e.field, 'field', 'name')
              .having((e) => e.requestId, 'requestId', 'r1')
              .having((e) => e.hasEnvelope, 'hasEnvelope', isTrue),
        ),
      );
      expect(reachable, [true]);
    });

    test('keeps the status of an answer that is not JSON', () async {
      final api = answering(
        (_) async => http.Response('<html>Bad Gateway</html>', 502),
      );
      await expectLater(
        api.getData('/me', (d) => d),
        throwsA(
          isA<ApiError>()
              .having((e) => e.status, 'status', 502)
              .having((e) => e.isNetwork, 'isNetwork', isFalse)
              .having((e) => e.hasEnvelope, 'hasEnvelope', isFalse)
              .having((e) => e.message, 'message', 'Request failed (502).'),
        ),
      );
    });

    test('reports a rejected token once', () async {
      final api = answering(
        (_) async => http.Response(
          jsonEncode({
            'error': 'Please sign in again.',
            'code': 'UNAUTHORIZED',
          }),
          401,
        ),
      );
      await expectLater(
        api.getData('/me', (d) => d),
        throwsA(isA<ApiError>().having((e) => e.isUnauthorized, '401', isTrue)),
      );
      expect(unauthorized, 1);
    });

    test('an empty answer is null', () async {
      final api = answering((_) async => http.Response('', 204));
      expect(await api.send('DELETE', '/groups/g1'), isNull);
    });

    test(
      'a GET serves the offline copy when the server is unreachable',
      () async {
        final online = answering(
          (_) async => http.Response(jsonEncode({'data': 'fresh'}), 200),
        );
        expect(await online.getData('/me', (d) => d), 'fresh');
        await pumpEventQueue();

        expect(await offline().getData('/me', (d) => d), 'fresh');
        expect(reachable, [true, false]);
      },
    );

    test('a GET without an offline copy fails', () async {
      await expectLater(offline().getData('/me', (d) => d), network());
      expect(reachable, [false]);
    });

    test('a write never reads the offline copy', () async {
      cache.entries['http://server.test/api/v1/groups'] = {'data': 'cached'};
      await expectLater(
        offline().postData('/groups', {'name': 'Cabin'}, (d) => d),
        network(),
      );
    });

    test('an error answer is not served from the offline copy', () async {
      cache.entries['http://server.test/api/v1/me'] = {'data': 'cached'};
      final api = answering(
        (_) async => http.Response(
          jsonEncode({'error': 'Not found.', 'code': 'NOT_FOUND'}),
          404,
        ),
      );
      await expectLater(
        api.getData('/me', (d) => d),
        throwsA(isA<ApiError>().having((e) => e.status, 'status', 404)),
      );
    });
  });

  test('normalizeServerUrl', () {
    const cases = {
      'splitspends.example': 'https://splitspends.example',
      '  https://splitspends.example  ': 'https://splitspends.example',
      'https://splitspends.example/': 'https://splitspends.example',
      'https://splitspends.example/api/v1/': 'https://splitspends.example',
      'https://splitspends.example/api/v1': 'https://splitspends.example',
      'http://localhost:8080//': 'http://localhost:8080',
      'https://example.com/money/': 'https://example.com/money',
      '': '',
    };
    cases.forEach((input, want) {
      expect(normalizeServerUrl(input), want, reason: '"$input"');
    });
  });
}
