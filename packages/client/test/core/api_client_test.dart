import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;

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

/// A body that starts and then loses its connection.
Stream<List<int>> _dropped() async* {
  yield utf8.encode('{"data": {"id": ');
  throw http.ClientException('Connection reset by peer');
}

/// A body that starts and then never finishes.
Stream<List<int>> _stalled() {
  final controller = StreamController<List<int>>();
  controller.add(utf8.encode('{"data": '));
  return controller.stream;
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
}
