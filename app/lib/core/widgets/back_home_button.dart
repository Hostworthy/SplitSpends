import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

/// Back button that still works when a page was opened from a link:
/// pops when possible, otherwise goes to the home screen.
class BackHomeButton extends StatelessWidget {
  const BackHomeButton({super.key, this.close = false});

  /// Use a close icon (for full-screen dialogs like the editor).
  final bool close;

  void _back(BuildContext context) =>
      context.canPop() ? context.pop() : context.go('/');

  @override
  Widget build(BuildContext context) => close
      ? CloseButton(onPressed: () => _back(context))
      : BackButton(onPressed: () => _back(context));
}
