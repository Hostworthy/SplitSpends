import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/money/money.dart';

/// Text field for money that reports minor units.
///
/// The text stays in the user's hands; [onChanged] receives the parsed value
/// in minor units (null when empty or invalid).
class AmountField extends ConsumerStatefulWidget {
  const AmountField({
    super.key,
    required this.currency,
    this.initial,
    this.onChanged,
    this.label,
    this.large = false,
    this.allowNegative = false,
    this.autofocus = false,
    this.textInputAction,
    this.onSubmitted,
  });

  final String currency;
  final int? initial;
  final ValueChanged<int?>? onChanged;
  final String? label;
  final bool large;
  final bool allowNegative;
  final bool autofocus;
  final TextInputAction? textInputAction;
  final VoidCallback? onSubmitted;

  @override
  ConsumerState<AmountField> createState() => _AmountFieldState();
}

class _AmountFieldState extends ConsumerState<AmountField> {
  late final TextEditingController _controller;
  String? _error;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || widget.initial == null) return;
      final exponent = ref
          .read(configValueProvider)
          .currency(widget.currency)
          .exponent;
      _controller.text = Money.toPlain(widget.initial!, exponent);
    });
  }

  @override
  void didUpdateWidget(AmountField oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.initial != widget.initial && widget.initial != null) {
      final exponent = ref
          .read(configValueProvider)
          .currency(widget.currency)
          .exponent;
      final current = Money.parse(_controller.text, exponent);
      if (current != widget.initial) {
        _controller.text = Money.toPlain(widget.initial!, exponent);
      }
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _changed(String text) {
    final info = ref.read(configValueProvider).currency(widget.currency);
    final value = Money.parse(text, info.exponent);
    setState(() {
      if (text.trim().isEmpty) {
        _error = null;
      } else if (value == null) {
        _error = 'Invalid amount';
      } else if (value < 0 && !widget.allowNegative) {
        _error = 'Must be positive';
      } else {
        _error = null;
      }
    });
    widget.onChanged?.call(_error == null ? value : null);
  }

  @override
  Widget build(BuildContext context) {
    final info = ref.watch(configValueProvider).currency(widget.currency);
    final theme = Theme.of(context);
    final style = widget.large
        ? theme.textTheme.headlineMedium?.copyWith(fontWeight: FontWeight.w700)
        : null;
    return TextField(
      controller: _controller,
      autofocus: widget.autofocus,
      onChanged: _changed,
      textInputAction: widget.textInputAction,
      onSubmitted: (_) => widget.onSubmitted?.call(),
      keyboardType: TextInputType.numberWithOptions(
        decimal: info.exponent > 0,
        signed: widget.allowNegative,
      ),
      inputFormatters: [
        FilteringTextInputFormatter.allow(
          RegExp(widget.allowNegative ? r'[0-9.,\-]' : r'[0-9.,]'),
        ),
      ],
      style: style,
      decoration: InputDecoration(
        labelText: widget.label,
        hintText: Money.toPlain(0, info.exponent),
        // Always visible (unlike prefixText), since the currency is set by
        // the group or settings and never picked next to the amount.
        prefixIcon: Padding(
          padding: const EdgeInsetsDirectional.only(start: 16, end: 6),
          child: Text(
            info.symbol,
            style: (style ?? theme.textTheme.bodyLarge)?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
        ),
        prefixIconConstraints: const BoxConstraints(),
        errorText: _error,
      ),
    );
  }
}
