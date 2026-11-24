import 'package:flutter/services.dart';

import 'package:splitspends/core/utils/share_stub.dart'
    if (dart.library.js_interop) 'package:splitspends/core/utils/share_web.dart'
    as impl;

/// How [shareText] delivered the text.
enum Shared { sheet, clipboard }

/// Opens the system share sheet where the platform offers one to the web
/// app (Web Share API); otherwise copies [text] to the clipboard so the
/// caller can say so.
Future<Shared> shareText(String text, {String? title}) async {
  if (await impl.shareSheet(text, title: title)) return Shared.sheet;
  await Clipboard.setData(ClipboardData(text: text));
  return Shared.clipboard;
}
