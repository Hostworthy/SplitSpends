import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/core/widgets/category_icon.dart';
import 'package:splitspends/core/widgets/pickers.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/app/repositories.dart';

/// Description, category, amount, currency, date, repeat, notes, receipt.
class DetailsSection extends ConsumerStatefulWidget {
  const DetailsSection({super.key, required this.form, this.onSubmit});

  final ExpenseForm form;
  final VoidCallback? onSubmit;

  @override
  ConsumerState<DetailsSection> createState() => _DetailsSectionState();
}

class _DetailsSectionState extends ConsumerState<DetailsSection> {
  late final _description = TextEditingController(
    text: widget.form.description,
  );
  late final _notes = TextEditingController(text: widget.form.notes);
  bool _uploading = false;

  ExpenseForm get form => widget.form;

  @override
  void dispose() {
    _description.dispose();
    _notes.dispose();
    super.dispose();
  }

  Future<void> _pickDate() async {
    final picked = await showDatePicker(
      context: context,
      initialDate: form.date,
      firstDate: DateTime(2000),
      lastDate: DateTime.now().add(const Duration(days: 365)),
    );
    if (picked != null) form.update(() => form.date = picked);
  }

  Future<void> _attachReceipt() async {
    final source = await showModalBottomSheet<ImageSource>(
      context: context,
      builder: (context) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.photo_camera_rounded),
              title: const Text('Take a photo'),
              onTap: () => Navigator.pop(context, ImageSource.camera),
            ),
            ListTile(
              leading: const Icon(Icons.photo_library_rounded),
              title: const Text('Choose from library'),
              onTap: () => Navigator.pop(context, ImageSource.gallery),
            ),
          ],
        ),
      ),
    );
    if (source == null) return;
    final file = await ImagePicker().pickImage(
      source: source,
      maxWidth: 2400,
      imageQuality: 85,
    );
    if (file == null) return;
    setState(() => _uploading = true);
    try {
      final Uint8List bytes = await file.readAsBytes();
      final receipt = await ref
          .read(receiptsRepositoryProvider)
          .upload(bytes, file.name, file.mimeType ?? 'image/jpeg');
      form.update(() => form.receiptId = receipt.id);
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final config = ref.watch(configValueProvider);
    final category = config.category(form.category);
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Tooltip(
                message: category?.name ?? 'Category',
                child: InkWell(
                  borderRadius: BorderRadius.circular(16),
                  onTap: () async {
                    final id = await pickCategory(
                      context,
                      selected: form.category,
                    );
                    if (id != null) form.update(() => form.category = id);
                  },
                  child: CategoryIcon(category: form.category, size: 56),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: TextField(
                  key: const Key('expense-description'),
                  controller: _description,
                  autofocus: form.description.isEmpty,
                  textCapitalization: TextCapitalization.sentences,
                  textInputAction: TextInputAction.next,
                  onChanged: (v) => form.update(() => form.description = v),
                  decoration: const InputDecoration(
                    labelText: 'Description',
                    hintText: 'Dinner, groceries, rent…',
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              SizedBox(
                width: 84,
                height: 64,
                child: OutlinedButton(
                  style: OutlinedButton.styleFrom(padding: EdgeInsets.zero),
                  onPressed: () async {
                    final code = await pickCurrency(
                      context,
                      selected: form.currency,
                    );
                    if (code != null) form.update(() => form.currency = code);
                  },
                  child: Text(form.currency),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: AmountField(
                  key: ValueKey('total-${form.currency}'),
                  currency: form.currency,
                  initial: form.total,
                  large: true,
                  onSubmitted: widget.onSubmit,
                  textInputAction: TextInputAction.done,
                  onChanged: (v) => form.update(() => form.total = v),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              ActionChip(
                avatar: const Icon(Icons.event_rounded, size: 18),
                label: Text(Dates.short(form.date)),
                onPressed: _pickDate,
              ),
              PopupMenuButton<Recurrence>(
                initialValue: form.recurrence,
                onSelected: (r) => form.update(() => form.recurrence = r),
                itemBuilder: (_) => [
                  for (final r in Recurrence.values)
                    PopupMenuItem(value: r, child: Text(r.label)),
                ],
                child: Chip(
                  avatar: const Icon(Icons.repeat_rounded, size: 18),
                  label: Text(
                    form.recurrence == Recurrence.none
                        ? 'Does not repeat'
                        : form.recurrence.label,
                  ),
                ),
              ),
              ActionChip(
                avatar: _uploading
                    ? const SizedBox.square(
                        dimension: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Icon(
                        form.receiptId == null
                            ? Icons.attach_file_rounded
                            : Icons.check_circle_rounded,
                        size: 18,
                      ),
                label: Text(
                  form.receiptId == null
                      ? 'Attach receipt'
                      : 'Receipt attached',
                ),
                onPressed: _uploading ? null : _attachReceipt,
              ),
            ],
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _notes,
            minLines: 1,
            maxLines: 4,
            onChanged: (v) => form.notes = v,
            decoration: const InputDecoration(
              labelText: 'Notes',
              prefixIcon: Icon(Icons.notes_rounded),
            ),
          ),
        ],
      ),
    );
  }
}
