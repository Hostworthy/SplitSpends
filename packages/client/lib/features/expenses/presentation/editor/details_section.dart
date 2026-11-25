import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/amount_field.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/categories/domain/catalog.dart';
import 'package:splitspends/features/categories/presentation/category_badge.dart';
import 'package:splitspends/features/categories/presentation/category_picker.dart';
import 'package:splitspends/features/categories/presentation/providers.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/expenses/presentation/editor/expense_form.dart';
import 'package:splitspends/features/receipts/presentation/upload_receipt.dart';

/// Description, category, amount, date, repeat, notes, receipt.
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
    final range = Dates.pickable(form.date);
    final picked = await showDatePicker(
      context: context,
      initialDate: form.date,
      firstDate: range.first,
      lastDate: range.last,
    );
    if (picked != null) form.update(() => form.date = picked);
  }

  Future<void> _attachReceipt() async {
    final source = await showModalBottomSheet<ImageSource>(
      useRootNavigator: true,
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
    if (source == null || !mounted) return;
    try {
      final receipt = await uploadReceipt(
        ref,
        source,
        onPicked: (_) {
          if (mounted) setState(() => _uploading = true);
        },
      );
      // The form is disposed with the editor, so leave it alone once this
      // section is gone.
      if (receipt == null || !mounted) return;
      form.update(() => form.receiptId = receipt.id);
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final category = ref
        .watch(categoryCatalogProvider)
        .find(form.category ?? generalCategory.id);
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Tooltip(
                message: category.name,
                child: InkWell(
                  key: const Key('expense-category'),
                  borderRadius: BorderRadius.circular(16),
                  onTap: () async {
                    final id = await pickCategory(
                      context,
                      selected: form.category,
                    );
                    if (id != null) form.update(() => form.category = id);
                  },
                  child: CategoryBadge(category: form.category, size: 56),
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
          // The currency comes from the group (or your Account settings for
          // expenses outside groups); it is never picked per expense.
          AmountField(
            currency: form.currency,
            initial: form.total,
            large: true,
            onSubmitted: widget.onSubmit,
            textInputAction: TextInputAction.done,
            onChanged: (v) => form.update(() => form.total = v),
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
              if (ref.watch(appModeProvider) == AppMode.connected)
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
