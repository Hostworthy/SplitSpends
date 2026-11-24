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

  /// Errors raised by on-device code, shaped like the server's.
  const ApiError.validation(String this.field, this.message)
    : status = 422,
      code = 'VALIDATION_ERROR',
      requestId = null;

  const ApiError.conflict(this.message)
    : status = 409,
      code = 'CONFLICT',
      field = null,
      requestId = null;

  ApiError.notFound(String what)
    : status = 404,
      code = 'NOT_FOUND',
      message = '$what not found.',
      field = null,
      requestId = null;

  /// A feature that needs a server connection.
  const ApiError.needsServer(this.message)
    : status = 501,
      code = 'NEEDS_SERVER',
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

  /// The account waits for an administrator or manager to approve it.
  bool get isPendingApproval => code == 'APPROVAL_PENDING';

  /// The request to join was declined.
  bool get isDeclined => code == 'APPROVAL_DECLINED';
  bool get needsServer => code == 'NEEDS_SERVER';

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
