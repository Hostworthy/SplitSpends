import 'dart:typed_data';

import 'package:splitspends/core/utils/download_stub.dart'
    if (dart.library.js_interop) 'package:splitspends/core/utils/download_web.dart'
    as impl;

/// Saves [bytes] as a file download on web. Returns false where downloads
/// are not supported (mobile), so callers can fall back (e.g. clipboard).
bool saveDownload(String filename, Uint8List bytes, String mimeType) =>
    impl.saveDownload(filename, bytes, mimeType);
