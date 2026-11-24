/// Error returned by the SplitSpends API (or raised for transport failures).
///
/// Mirrors the error envelope `{ error, code, field?, requestId? }`.
class ApiError implements Exception {
  const ApiError({
    required this.status,
    required this.message,
    this.code = 'INTERNAL',
    this.field,
    this.requestId,
  });

  /// Network or parsing failure (no HTTP status).
  const ApiError.network(this.message)
    : status = 0,
      code = 'NETWORK',
      field = null,
      requestId = null;

  factory ApiError.fromJson(int status, Map<String, dynamic> json) {
    return ApiError(
      status: status,
      message: (json['error'] as String?) ?? 'Something went wrong.',
      code: (json['code'] as String?) ?? _codeFor(status),
      field: json['field'] as String?,
      requestId: json['requestId'] as String?,
    );
  }

  final int status;
  final String message;
  final String code;
  final String? field;
  final String? requestId;

  bool get isUnauthorized => status == 401;
  bool get isNetwork => status == 0;
  bool get isAiDisabled => code == 'AI_DISABLED';

  static String _codeFor(int status) => switch (status) {
    401 => 'UNAUTHORIZED',
    403 => 'FORBIDDEN',
    404 => 'NOT_FOUND',
    409 => 'CONFLICT',
    422 => 'VALIDATION_ERROR',
    429 => 'RATE_LIMITED',
    503 => 'AI_DISABLED',
    _ => 'INTERNAL',
  };

  @override
  String toString() => message;
}

/// Human friendly message for any thrown error.
String describeError(Object error) {
  if (error is ApiError) {
    if (error.isNetwork) {
      return 'Cannot reach the server. Check your connection.';
    }
    return error.message;
  }
  return 'Something went wrong.';
}
