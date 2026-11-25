import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/theme/theme.dart';
import 'package:splitspends/core/models/user.dart';

/// Circular avatar with initials on the user's color.
class UserAvatar extends StatelessWidget {
  const UserAvatar({super.key, required this.user, this.size = 40});

  final User user;
  final double size;

  @override
  Widget build(BuildContext context) {
    final color = parseHexColor(user.avatarColor) ?? colorFor(user.id);
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: color,
        border: user.isInvited
            ? Border.all(
                color: Theme.of(context).colorScheme.outline,
                width: 1.5,
                strokeAlign: BorderSide.strokeAlignOutside,
              )
            : null,
      ),
      child: _picture(
        Text(
          user.initials,
          style: TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w700,
            fontSize: size * 0.38,
          ),
        ),
      ),
    );
  }

  /// The Gravatar picture over the initials, which stay visible while it
  /// loads and when the person has none (Gravatar answers 404).
  Widget _picture(Widget initials) {
    final url = user.avatarUrl;
    if (url == null) return initials;
    return Stack(
      alignment: Alignment.center,
      children: [
        initials,
        ClipOval(
          child: Image.network(
            url,
            width: size,
            height: size,
            fit: BoxFit.cover,
            excludeFromSemantics: true,
            frameBuilder: (context, child, frame, sync) =>
                frame == null ? const SizedBox.shrink() : child,
            errorBuilder: (context, error, stack) => const SizedBox.shrink(),
          ),
        ),
      ],
    );
  }

  /// Stable palette color for an id.
  static Color colorFor(String id) {
    var hash = 0;
    for (final unit in id.codeUnits) {
      hash = (hash * 31 + unit) & 0x7fffffff;
    }
    return palette[hash % palette.length];
  }
}

/// Overlapping avatars for a group's members.
class AvatarStack extends StatelessWidget {
  const AvatarStack({
    super.key,
    required this.users,
    this.size = 28,
    this.max = 5,
  });

  final List<User> users;
  final double size;
  final int max;

  @override
  Widget build(BuildContext context) {
    final shown = users.take(max).toList();
    final extra = users.length - shown.length;
    final step = size * 0.68;
    final count = shown.length + (extra > 0 ? 1 : 0);
    final surface = Theme.of(context).colorScheme.surface;
    return SizedBox(
      height: size + 4,
      width: count == 0 ? 0 : step * (count - 1) + size + 4,
      child: Stack(
        children: [
          for (var i = 0; i < shown.length; i++)
            Positioned(
              left: i * step,
              child: Container(
                padding: const EdgeInsets.all(2),
                decoration: BoxDecoration(
                  color: surface,
                  shape: BoxShape.circle,
                ),
                child: UserAvatar(user: shown[i], size: size),
              ),
            ),
          if (extra > 0)
            Positioned(
              left: shown.length * step,
              child: Container(
                padding: const EdgeInsets.all(2),
                decoration: BoxDecoration(
                  color: surface,
                  shape: BoxShape.circle,
                ),
                child: CircleAvatar(
                  radius: size / 2,
                  child: Text(
                    '+$extra',
                    style: TextStyle(fontSize: size * 0.36),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
