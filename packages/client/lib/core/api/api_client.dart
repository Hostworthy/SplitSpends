import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/paging/page_result.dart';

export 'package:splitspends/core/models/json.dart';
export 'package:splitspends/core/paging/page_result.dart';

/// Thin HTTP client for the SplitSpends API.
///
/// Handles the bearer token, the `{ data }` / `{ items, total }` envelopes and
/// the error envelope. Feature repositories build on top of it.
class ApiClient {
  ApiClient({
    required this.serverUrl,
    this.token,
    http.Client? httpClient,
    this.onUnauthorized,
    this.cache,
    this.onReachability,
    this.timeout = const Duration(seconds: 30),
  }) : _http = httpClient ?? http.Client();

  final String serverUrl;
  final String? token;
  final void Function()? onUnauthorized;
  final Duration timeout;
  final http.Client _http;

  /// Stores successful GET responses and serves them when offline.
  final ResponseCache? cache;

  /// Called with true after a request reached the server, false when it
  /// could not (network failure).
  final void Function(bool reachable)? onReachability;

  String get apiBase => '${normalizeServerUrl(serverUrl)}/api/v1';

  Uri uri(String path, [Map<String, Object?>? query]) {
    final params = <String, String>{};
    query?.forEach((key, value) {
      if (value == null) return;
      final text = value.toString();
      if (text.isEmpty) return;
      params[key] = text;
    });
    final base = Uri.parse('$apiBase$path');
    return params.isEmpty ? base : base.replace(queryParameters: params);
  }

  Map<String, String> headers({bool json = true}) => {
    if (json) 'Content-Type': 'application/json',
    'Accept': 'application/json',
    if (token != null) 'Authorization': 'Bearer $token',
  };

  /// GET returning the `data` field of a details envelope.
  Future<T> getData<T>(
    String path,
    T Function(Object? data) parse, {
    Map<String, Object?>? query,
  }) async {
    final body = await _send('GET', path, query: query);
    return parse((body as JsonMap)['data']);
  }

  /// GET returning a list envelope.
  Future<PageResult<T>> getList<T>(
    String path,
    T Function(JsonMap item) parse, {
    Map<String, Object?>? query,
  }) async {
    final body = await _send('GET', path, query: query) as JsonMap;
    final items = (body['items'] as List? ?? const [])
        .cast<JsonMap>()
        .map(parse)
        .toList();
    return PageResult(
      items: items,
      total: (body['total'] as num?)?.toInt() ?? items.length,
    );
  }

  Future<T> postData<T>(
    String path,
    Object? payload,
    T Function(Object? data) parse,
  ) async {
    final body = await _send('POST', path, payload: payload);
    return parse((body as JsonMap?)?['data']);
  }

  Future<T> putData<T>(
    String path,
    Object? payload,
    T Function(Object? data) parse,
  ) async {
    final body = await _send('PUT', path, payload: payload);
    return parse((body as JsonMap)['data']);
  }

  Future<T> patchData<T>(
    String path,
    Object? payload,
    T Function(Object? data) parse,
  ) async {
    final body = await _send('PATCH', path, payload: payload);
    return parse((body as JsonMap)['data']);
  }

  Future<void> post(String path, [Object? payload]) =>
      _send('POST', path, payload: payload);

  /// Sends a request as-is (the offline outbox replays these).
  Future<Object?> send(String method, String path, {Object? payload}) =>
      _send(method, path, payload: payload);

  Future<void> delete(String path) => _send('DELETE', path);

  /// Fetches raw bytes (receipt images, CSV export).
  Future<Uint8List> getBytes(String path) async {
    final response = await _guard(
      () => _http.get(uri(path), headers: headers(json: false)),
    );
    _check(response);
    return response.bodyBytes;
  }

  /// Uploads a single file as `multipart/form-data`.
  Future<T> upload<T>(
    String path, {
    required String field,
    required Uint8List bytes,
    required String filename,
    required String contentType,
    required T Function(Object? data) parse,
  }) async {
    final request = http.MultipartRequest('POST', uri(path))
      ..headers.addAll(headers(json: false))
      ..files.add(
        http.MultipartFile.fromBytes(
          field,
          bytes,
          filename: filename,
          contentType: _mediaType(contentType),
        ),
      );
    final response = await _guard(
      () async => http.Response.fromStream(await _http.send(request)),
    );
    _check(response);
    return parse((jsonDecode(response.body) as JsonMap)['data']);
  }

  Future<Object?> _send(
    String method,
    String path, {
    Object? payload,
    Map<String, Object?>? query,
  }) async {
    final target = uri(path, query);
    final cacheKey = method == 'GET' && cache != null ? '$target' : null;
    final request = http.Request(method, target)
      ..headers.addAll(headers(json: payload != null));
    if (payload != null) request.body = jsonEncode(payload);
    http.Response response;
    try {
      // The body is read inside the guard too: a connection that drops or
      // stalls mid-body is a network failure like any other.
      response = await _guard(
        () async => http.Response.fromStream(await _http.send(request)),
      );
    } on ApiError catch (error) {
      if (error.isNetwork) {
        onReachability?.call(false);
        if (cacheKey != null) {
          final hit = await cache!.read(cacheKey);
          if (hit != null) return hit;
        }
      }
      rethrow;
    }
    onReachability?.call(true);
    _check(response);
    if (response.statusCode == 204 || response.body.isEmpty) return null;
    final Object? decoded;
    try {
      decoded = jsonDecode(utf8.decode(response.bodyBytes));
    } on FormatException {
      throw const ApiError.network('The server sent an unexpected response.');
    }
    if (cacheKey != null) unawaited(cache!.write(cacheKey, decoded));
    return decoded;
  }

  Future<R> _guard<R>(Future<R> Function() run) async {
    try {
      return await run().timeout(timeout);
    } on ApiError {
      rethrow;
    } on TimeoutException {
      throw const ApiError.network('The server took too long to respond.');
    } catch (_) {
      throw const ApiError.network('Cannot reach the server.');
    }
  }

  void _check(http.Response response) {
    if (response.statusCode >= 200 && response.statusCode < 300) return;
    final retryAfter = _retryAfter(response.headers['retry-after']);
    ApiError error;
    try {
      final decoded = jsonDecode(utf8.decode(response.bodyBytes));
      error = ApiError.fromJson(
        response.statusCode,
        decoded as JsonMap,
        retryAfter: retryAfter,
      );
    } catch (_) {
      error = ApiError(
        status: response.statusCode,
        message: 'Request failed (${response.statusCode}).',
        retryAfter: retryAfter,
      );
    }
    if (error.isUnauthorized) onUnauthorized?.call();
    throw error;
  }
}

/// Removes trailing slashes and a trailing `/api/v1` from a user-entered URL.
String normalizeServerUrl(String url) {
  var value = url.trim();
  while (value.endsWith('/')) {
    value = value.substring(0, value.length - 1);
  }
  if (value.endsWith('/api/v1')) {
    value = value.substring(0, value.length - '/api/v1'.length);
  }
  if (value.isNotEmpty && !value.contains('://')) value = 'https://$value';
  return value;
}

/// Reads a Retry-After header: delay seconds or an HTTP date.
Duration? _retryAfter(String? value) {
  if (value == null) return null;
  final seconds = int.tryParse(value.trim());
  if (seconds != null) return seconds < 0 ? null : Duration(seconds: seconds);
  try {
    final wait = parseHttpDate(value).difference(DateTime.now().toUtc());
    return wait.isNegative ? Duration.zero : wait;
  } on FormatException {
    return null;
  }
}

MediaType? _mediaType(String contentType) {
  try {
    return MediaType.parse(contentType);
  } on FormatException {
    return null;
  }
}

/// Offline copies of GET responses, keyed by URL.
abstract interface class ResponseCache {
  Future<Object?> read(String key);
  Future<void> write(String key, Object? body);
  Future<void> clear();
}
