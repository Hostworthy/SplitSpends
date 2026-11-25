import 'package:file_picker/file_picker.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/features/sync/data/providers.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/states.dart';

/// Exports the on-device ledger and lets the person save the file.
Future<void> saveBackup(BuildContext context, WidgetRef ref) async {
  try {
    final bytes = await ref.read(backupServiceProvider).export();
    final name = 'splitspends-backup-${Dates.toApi(DateTime.now())}.json';
    await FilePicker.saveFile(
      fileName: name,
      bytes: bytes,
      mimeType: 'application/json',
      dialogTitle: 'Save backup',
    );
  } catch (error) {
    if (context.mounted) showError(context, error);
  }
}
