import 'dart:js_interop';
import 'dart:js_interop_unsafe';

import 'package:web/web.dart' as web;

/// Uses the Web Share API when the browser has it (mostly phones). Returns
/// false when it is missing so the caller copies the text instead.
Future<bool> shareSheet(String text, {String? title}) async {
  final navigator = web.window.navigator;
  if (!navigator.has('share')) return false;
  final data = web.ShareData(text: text, title: title ?? '');
  if (navigator.has('canShare') && !navigator.canShare(data)) return false;
  try {
    await navigator.share(data).toDart;
  } catch (_) {
    // Dismissing the sheet rejects the promise; that is not a failure.
  }
  return true;
}
