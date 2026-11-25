import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/theme/theme.dart';

/// The palette as round swatches, each announced by its colour name and
/// whether it is the chosen one. [selected] and the value passed to
/// [onSelected] are `#RRGGBB`.
class Swatches extends StatelessWidget {
  const Swatches({
    super.key,
    required this.selected,
    required this.onSelected,
    this.radius = 16,
  });

  final String? selected;
  final ValueChanged<String> onSelected;
  final double radius;

  @override
  Widget build(BuildContext context) => Wrap(
    spacing: 10,
    runSpacing: 10,
    children: [
      for (final s in swatches)
        _Swatch(
          name: s.name,
          color: s.color,
          radius: radius,
          chosen: selected?.toUpperCase() == toHexColor(s.color),
          onTap: () => onSelected(toHexColor(s.color)),
        ),
    ],
  );
}

class _Swatch extends StatelessWidget {
  const _Swatch({
    required this.name,
    required this.color,
    required this.radius,
    required this.chosen,
    required this.onTap,
  });

  final String name;
  final Color color;
  final double radius;
  final bool chosen;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => Tooltip(
    message: name,
    excludeFromSemantics: true,
    child: Semantics(
      button: true,
      selected: chosen,
      label: '$name color',
      child: InkWell(
        customBorder: const CircleBorder(),
        onTap: onTap,
        child: CircleAvatar(
          radius: radius,
          backgroundColor: color,
          child: chosen
              ? Icon(Icons.check, color: Colors.white, size: radius + 2)
              : null,
        ),
      ),
    ),
  );
}
