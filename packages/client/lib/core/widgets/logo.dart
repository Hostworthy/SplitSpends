import 'package:material_ui/material_ui.dart';

/// The app mark, drawn like the app icon (packages/brand/icon.svg): a white
/// and a mint coin overlapping on a teal rounded square.
class AppLogo extends StatelessWidget {
  const AppLogo({super.key, this.size = 48});

  final double size;

  @override
  Widget build(BuildContext context) => Semantics(
    label: 'Split Spends',
    image: true,
    child: CustomPaint(size: Size.square(size), painter: const _LogoPainter()),
  );
}

class _LogoPainter extends CustomPainter {
  const _LogoPainter();

  // Geometry of the 1024-unit icon in packages/brand/icon.svg.
  static const _left = Offset(392 / 1024, 0.5);
  static const _right = Offset(632 / 1024, 0.5);
  static const _radius = 232 / 1024;
  static const _cut = 272 / 1024;
  static const _ring = 167 / 1024;
  static const _stroke = 16 / 1024;

  @override
  void paint(Canvas canvas, Size size) {
    final s = size.width;
    final square = RRect.fromRectAndRadius(
      Offset.zero & size,
      Radius.circular(s * 0.225),
    );
    canvas.drawRRect(
      square,
      Paint()
        ..shader = const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF12907A), Color(0xFF0A5E4E)],
        ).createShader(Offset.zero & size),
    );
    Path circle(Offset c, double r) =>
        Path()..addOval(Rect.fromCircle(center: c * s, radius: r * s));
    final left = Path.combine(
      PathOperation.difference,
      circle(_left, _radius),
      circle(_right, _cut),
    );
    canvas.drawPath(left, Paint()..color = const Color(0xFFFFFFFF));
    canvas.drawCircle(
      _right * s,
      _radius * s,
      Paint()..color = const Color(0xFF9DEBCF),
    );
    if (s >= 28) {
      final ring = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = _stroke * s;
      canvas
        ..save()
        ..clipPath(left)
        ..drawCircle(
          _left * s,
          _ring * s,
          ring..color = const Color(0xFFD6EFE6),
        )
        ..restore()
        ..drawCircle(
          _right * s,
          _ring * s,
          ring..color = const Color(0xFF6FD3B2),
        );
    }
  }

  @override
  bool shouldRepaint(_LogoPainter oldDelegate) => false;
}
