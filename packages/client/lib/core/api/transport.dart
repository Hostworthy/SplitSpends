import 'package:splitspends/core/api/transport_io.dart'
    if (dart.library.js_interop) 'package:splitspends/core/api/transport_web.dart'
    as impl;

/// True when [error] means the connection failed: no route, refused,
/// dropped or reset. The browser reports these as `ClientException`.
bool isTransportFailure(Object error) => impl.isTransportFailure(error);

/// True when the server's TLS certificate or handshake was rejected. The
/// browser does not tell these apart from other connection failures.
bool isTlsFailure(Object error) => impl.isTlsFailure(error);
