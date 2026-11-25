import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';

import 'package:splitspends/features/receipts/data/providers.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';

/// The device camera and photo library; replaced in tests.
final imagePickerProvider = Provider<ImagePicker>((ref) => ImagePicker());

/// Lets the person take or choose a photo and uploads it as a receipt.
/// Returns null when they cancel. [onPicked] gets the photo once it is
/// read, before the upload starts. Every failure (a denied camera, an
/// unreadable file, the upload) is thrown for the caller to show.
Future<Receipt?> uploadReceipt(
  WidgetRef ref,
  ImageSource source, {
  void Function(Uint8List bytes)? onPicked,
}) async {
  // Read before the first await: the caller may be gone by the time the
  // picker returns.
  final picker = ref.read(imagePickerProvider);
  final receipts = ref.read(receiptsRepositoryProvider);
  final file = await picker.pickImage(
    source: source,
    maxWidth: 2400,
    imageQuality: 85,
  );
  if (file == null) return null;
  final bytes = await file.readAsBytes();
  onPicked?.call(bytes);
  return receipts.upload(bytes, file.name, file.mimeType ?? 'image/jpeg');
}
