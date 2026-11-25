import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/widgets/skeleton.dart';
import 'package:splitspends/features/receipts/data/providers.dart';

/// Receipt bytes, fetched with the bearer token.
final receiptBytesProvider = FutureProvider.family<Uint8List, String>(
  (ref, id) => ref.watch(receiptsRepositoryProvider).image(id),
);

class ReceiptImage extends ConsumerWidget {
  const ReceiptImage({super.key, required this.receiptId});

  final String receiptId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final bytes = ref.watch(receiptBytesProvider(receiptId));
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 12, 16, 0),
      child: switch (bytes) {
        AsyncValue(:final value?) => InkWell(
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute<void>(
              fullscreenDialog: true,
              builder: (_) => Scaffold(
                backgroundColor: Colors.black,
                appBar: AppBar(
                  backgroundColor: Colors.black,
                  foregroundColor: Colors.white,
                ),
                body: InteractiveViewer(
                  maxScale: 5,
                  child: Center(
                    child: Hero(
                      tag: 'receipt-$receiptId',
                      child: Image.memory(value),
                    ),
                  ),
                ),
              ),
            ),
          ),
          child: Hero(
            tag: 'receipt-$receiptId',
            child: Image.memory(
              value,
              height: 220,
              width: double.infinity,
              fit: BoxFit.cover,
            ),
          ),
        ),
        AsyncValue(:final error?) => ListTile(
          leading: const Icon(Icons.broken_image_outlined),
          title: const Text('Receipt unavailable'),
          subtitle: Text('$error'),
        ),
        _ => const Skeleton(height: 220, radius: 0),
      },
    );
  }
}
