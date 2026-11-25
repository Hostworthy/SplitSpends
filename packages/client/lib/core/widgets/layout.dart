import 'package:material_ui/material_ui.dart';

/// Width at which the app switches to rail + two panes.
const wideBreakpoint = 700.0;

bool isWide(BuildContext context) =>
    MediaQuery.sizeOf(context).width >= wideBreakpoint;
