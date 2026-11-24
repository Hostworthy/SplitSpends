import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

/// Six boxes backed by one invisible text field, so paste and SMS/email
/// autofill work naturally. Calls [onCompleted] when all digits are in.
class OtpInput extends StatefulWidget {
  const OtpInput({
    super.key,
    required this.onCompleted,
    this.length = 6,
    this.enabled = true,
    this.controller,
  });

  final ValueChanged<String> onCompleted;
  final int length;
  final bool enabled;
  final TextEditingController? controller;

  @override
  State<OtpInput> createState() => _OtpInputState();
}

class _OtpInputState extends State<OtpInput> {
  late final TextEditingController _controller =
      widget.controller ?? TextEditingController();
  final _focus = FocusNode();

  @override
  void initState() {
    super.initState();
    _controller.addListener(_changed);
  }

  @override
  void dispose() {
    _controller.removeListener(_changed);
    if (widget.controller == null) _controller.dispose();
    _focus.dispose();
    super.dispose();
  }

  void _changed() {
    setState(() {});
    final value = _controller.text;
    if (value.length == widget.length) {
      HapticFeedback.lightImpact();
      widget.onCompleted(value);
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final text = _controller.text;
    return GestureDetector(
      onTap: _focus.requestFocus,
      child: Stack(
        alignment: Alignment.center,
        children: [
          // Invisible but reachable by screen readers and autofill.
          Opacity(
            opacity: 0,
            alwaysIncludeSemantics: true,
            child: TextField(
              key: const Key('otp-field'),
              controller: _controller,
              focusNode: _focus,
              autofocus: true,
              enabled: widget.enabled,
              keyboardType: TextInputType.number,
              autofillHints: const [AutofillHints.oneTimeCode],
              showCursor: false,
              maxLength: widget.length,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(
                counterText: '',
                labelText: 'Verification code',
              ),
            ),
          ),
          IgnorePointer(
            child: ListenableBuilder(
              listenable: _focus,
              builder: (context, _) => Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  for (var i = 0; i < widget.length; i++)
                    _Box(
                      char: i < text.length ? text[i] : '',
                      active:
                          _focus.hasFocus &&
                          (i == text.length ||
                              (i == widget.length - 1 &&
                                  text.length == widget.length)),
                      scheme: scheme,
                    ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _Box extends StatelessWidget {
  const _Box({required this.char, required this.active, required this.scheme});

  final String char;
  final bool active;
  final ColorScheme scheme;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: const Duration(milliseconds: 150),
      width: 48,
      height: 58,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: scheme.surfaceContainerHighest.withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(
          color: active ? scheme.primary : Colors.transparent,
          width: 2,
        ),
      ),
      child: Text(
        char,
        style: Theme.of(context).textTheme.headlineSmall
            ?.copyWith(fontWeight: FontWeight.w700),
      ),
    );
  }
}
