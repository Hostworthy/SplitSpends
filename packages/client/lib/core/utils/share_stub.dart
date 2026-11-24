/// No share sheet without a platform plugin; callers fall back to copying.
Future<bool> shareSheet(String text, {String? title}) async => false;
