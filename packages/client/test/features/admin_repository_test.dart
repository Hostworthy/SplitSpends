import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/features/admin/data/remote_admin_repository.dart';

void main() {
  test(
    'requests reads every page, so the oldest people waiting show',
    () async {
      const waiting = 450;
      final offsets = <String>[];
      final api = ApiClient(
        serverUrl: 'http://server.test',
        token: 't',
        httpClient: MockClient((request) async {
          final query = request.url.queryParameters;
          final limit = int.parse(query['limit']!);
          final offset = int.parse(query['offset'] ?? '0');
          offsets.add('$offset');
          final end = offset + limit < waiting ? offset + limit : waiting;
          return http.Response(
            jsonEncode({
              'items': [
                for (var i = offset; i < end; i++)
                  {
                    'id': 'r$i',
                    'email': 'person$i@else.org',
                    'status': 'pending',
                    'requestedAt': '2026-11-24T12:00:00Z',
                  },
              ],
              'total': waiting,
            }),
            200,
          );
        }),
      );

      final requests = await RemoteAdminRepository(api).requests();

      expect(requests, hasLength(waiting));
      expect(requests.last.email, 'person449@else.org');
      expect(offsets, ['0', '200', '400']);
    },
  );
}
