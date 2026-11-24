import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:image_picker/image_picker.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/routes.dart';
import 'package:splitspends/core/widgets/back_home_button.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/groups/presentation/providers.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';

enum _Step { pick, uploading, reading, failed }

/// Photo → upload → AI scan → itemized expense editor for review.
class ScanReceiptScreen extends ConsumerStatefulWidget {
  const ScanReceiptScreen({super.key, this.groupId});

  final String? groupId;

  @override
  ConsumerState<ScanReceiptScreen> createState() => _ScanReceiptScreenState();
}

class _ScanReceiptScreenState extends ConsumerState<ScanReceiptScreen> {
  _Step _step = _Step.pick;
  Uint8List? _image;
  String? _error;

  Future<void> _pick(ImageSource source) async {
    final file = await ImagePicker().pickImage(
      source: source,
      maxWidth: 2400,
      imageQuality: 85,
    );
    if (file == null) return;
    final bytes = await file.readAsBytes();
    setState(() {
      _image = bytes;
      _step = _Step.uploading;
      _error = null;
    });
    final repo = ref.read(receiptsRepositoryProvider);
    try {
      final receipt = await repo.upload(
        bytes,
        file.name,
        file.mimeType ?? 'image/jpeg',
      );
      if (!mounted) return;
      setState(() => _step = _Step.reading);
      final currency = widget.groupId == null
          ? ref.read(meProvider)?.defaultCurrency
          : ref.read(groupProvider(widget.groupId!)).value?.defaultCurrency;
      final scan = await repo.scan(receipt.id, currency: currency);
      if (!mounted) return;
      context.pushReplacement(
        Routes.newExpense,
        extra: ExpenseEditorArgs(
          groupId: widget.groupId,
          draft: _draft(scan, receipt.id),
        ),
      );
    } on ApiError catch (error) {
      setState(() {
        _step = _Step.failed;
        _error = error.isAiDisabled
            ? 'AI receipt scanning is not enabled on this server.'
            : describeError(error);
      });
    }
  }

  ExpenseInput _draft(ReceiptScan scan, String receiptId) {
    final meId = ref.read(meProvider)?.id ?? '';
    return ExpenseInput(
      groupId: widget.groupId,
      description: scan.merchant ?? 'Receipt',
      currency: scan.currency,
      total: scan.total,
      date: scan.date ?? Dates.toApi(Dates.today()),
      category: scan.category,
      splitMethod: SplitMethod.itemized,
      payers: [Payer(userId: meId, amount: scan.total)],
      items: scan.items,
      tax: scan.tax,
      tip: scan.tip,
      receiptId: receiptId,
    );
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final busy = _step == _Step.uploading || _step == _Step.reading;
    return Scaffold(
      appBar: AppBar(
        leading: const BackHomeButton(close: true),
        title: const Text('Scan a receipt'),
      ),
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: MaxWidth(
            width: 460,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                AspectRatio(
                  aspectRatio: 3 / 4,
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(24),
                    child: Stack(
                      fit: StackFit.expand,
                      children: [
                        _image == null
                            ? Container(
                                color: scheme.surfaceContainerHigh,
                                child: Icon(
                                  Icons.receipt_long_rounded,
                                  size: 96,
                                  color: scheme.outline,
                                ),
                              )
                            : Image.memory(_image!, fit: BoxFit.cover),
                        if (busy)
                          Container(
                            color: Colors.black.withValues(alpha: 0.45),
                            child: Column(
                              mainAxisAlignment: MainAxisAlignment.center,
                              children: [
                                const CircularProgressIndicator(
                                  color: Colors.white,
                                ),
                                const SizedBox(height: 16),
                                Text(
                                  _step == _Step.uploading
                                      ? 'Uploading…'
                                      : 'Reading the receipt…',
                                  style: text.titleMedium?.copyWith(
                                    color: Colors.white,
                                  ),
                                ),
                              ],
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 16),
                Text(
                  _error ??
                      'Take a clear photo of the whole receipt. AI reads the items, '
                          'tax and tip, then you assign items to people.',
                  style: text.bodyMedium?.copyWith(
                    color: _error == null ? null : scheme.error,
                  ),
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 24),
                if (!kIsWeb)
                  FilledButton.icon(
                    onPressed: busy ? null : () => _pick(ImageSource.camera),
                    icon: const Icon(Icons.photo_camera_rounded),
                    label: const Text('Take photo'),
                  ),
                const SizedBox(height: 12),
                OutlinedButton.icon(
                  onPressed: busy ? null : () => _pick(ImageSource.gallery),
                  icon: const Icon(Icons.photo_library_rounded),
                  label: Text(
                    kIsWeb ? 'Choose a photo' : 'Choose from library',
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
