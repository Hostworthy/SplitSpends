import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

/// Group icons by theme. Any other emoji can be typed in the picker.
const groupEmojis = <String, List<String>>{
  'Home': [
    '🏠',
    '🏡',
    '🏘️',
    '🏢',
    '🏬',
    '🛋️',
    '🛏️',
    '🚿',
    '🧺',
    '🧹',
    '🔑',
    '🪴', //
    '💡', '🔌', '📦', '🧾', '🛒', '🧸', '🐶', '🐱',
  ],
  'Travel': [
    '✈️',
    '🧳',
    '🗺️',
    '🧭',
    '🏖️',
    '🏝️',
    '🏔️',
    '⛰️',
    '🏕️',
    '⛺',
    '🗽',
    '🗼', //
    '🏰', '🌋', '🛳️', '🚢', '🚆', '🚄', '🚌', '🚗', '🚙', '🏍️', '🚲', '🛵',
    '🚀', '🌍', '🌎', '🌏',
  ],
  'Food and drink': [
    '🍕', '🍔', '🌮', '🌯', '🍣', '🍜', '🍝', '🥗', '🥘', '🍲', '🥐', '🥞', //
    '🧇', '🍳', '🍩', '🍰', '🎂', '🍦', '☕', '🍵', '🧋', '🍺', '🍷', '🥂',
    '🍸', '🍹', '🥤', '🧃',
  ],
  'Fun': [
    '🎉', '🎊', '🥳', '🎁', '🎈', '🎄', '🎃', '🎆', '🎤', '🎧', '🎸', '🎮', //
    '🎲', '🧩', '🎬', '🎟️', '🎭', '🎨', '📷', '🎳', '🎯', '🪩',
  ],
  'Sports': [
    '⚽', '🏀', '🏈', '⚾', '🎾', '🏐', '🏓', '🏸', '⛳', '🏒', '🥊', '🏋️', //
    '🚴', '🏃', '🧗', '🏄', '🏊', '⛷️', '🏂', '🛶', '🧘', '🏆',
  ],
  'Work and study': [
    '💼', '🏢', '💻', '🖥️', '📱', '📊', '📈', '🗂️', '📝', '📚', '🎓', '🏫', //
    '🔬', '🧪', '🛠️', '🔧', '🏗️', '💰', '💳', '🏦',
  ],
  'People': [
    '👥',
    '👫',
    '👭',
    '👬',
    '👪',
    '👨‍👩‍👧',
    '💞',
    '❤️',
    '💍',
    '💒',
    '👶',
    '🧑‍🤝‍🧑', //
    '🤝', '🙌', '👋', '😎', '🤓', '🥰', '😂', '🤗',
  ],
  'Nature': [
    '🌳', '🌲', '🌴', '🌵', '🌻', '🌸', '🌷', '🍀', '🍁', '🌊', '☀️', '🌙', //
    '⭐', '🌈', '❄️', '🔥', '⚡', '🌿', '🐾', '🦋',
  ],
  'Symbols': [
    '⭐', '✨', '💎', '🔔', '📌', '🏷️', '🧡', '💛', '💚', '💙', '💜', '🖤', //
    '🤍', '🔴', '🟠', '🟡', '🟢', '🔵', '🟣', '⚫',
  ],
};

/// Shown inline in group settings; the rest is one tap away.
const quickEmojis = [
  '🏠', '✈️', '💞', '👥', '🏖️', '🎉', '🍕', '🚗', '🏕️', '⚽', '🎓', '💼', //
];

/// Opens the full picker. Returns the chosen emoji, or null.
Future<String?> pickEmoji(BuildContext context, {String? selected}) =>
    showModalBottomSheet<String>(
      context: context,
      useRootNavigator: true,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (context) => _EmojiSheet(selected: selected),
    );

class _EmojiSheet extends StatefulWidget {
  const _EmojiSheet({this.selected});

  final String? selected;

  @override
  State<_EmojiSheet> createState() => _EmojiSheetState();
}

class _EmojiSheetState extends State<_EmojiSheet> {
  final _custom = TextEditingController();

  @override
  void dispose() {
    _custom.dispose();
    super.dispose();
  }

  /// The first character the person typed or pasted, as one emoji.
  String? get _typed {
    final text = _custom.text.trim();
    if (text.isEmpty) return null;
    final first = text.characters.first;
    // Plain letters and digits are not icons.
    return RegExp(r'^[\p{L}\p{N}\p{P}\s]+$', unicode: true).hasMatch(first)
        ? null
        : first;
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final typed = _typed;
    return DraggableScrollableSheet(
      expand: false,
      initialChildSize: 0.7,
      maxChildSize: 0.95,
      builder: (context, controller) => ListView(
        controller: controller,
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
        children: [
          Text('Group icon', style: text.titleLarge),
          const SizedBox(height: 12),
          TextField(
            key: const Key('emoji-custom'),
            controller: _custom,
            inputFormatters: [LengthLimitingTextInputFormatter(16)],
            onChanged: (_) => setState(() {}),
            onSubmitted: (_) {
              if (typed != null) Navigator.pop(context, typed);
            },
            decoration: InputDecoration(
              labelText: 'Or type any emoji',
              helperText: 'Use the emoji key on your keyboard.',
              suffixIcon: typed == null
                  ? null
                  : IconButton(
                      tooltip: 'Use $typed',
                      icon: const Icon(Icons.check_rounded),
                      onPressed: () => Navigator.pop(context, typed),
                    ),
            ),
          ),
          for (final entry in groupEmojis.entries) ...[
            const SizedBox(height: 16),
            Text(
              entry.key,
              style: text.labelLarge?.copyWith(color: scheme.primary),
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 4,
              runSpacing: 4,
              children: [
                for (final e in entry.value)
                  Semantics(
                    button: true,
                    selected: e == widget.selected,
                    label: 'Icon $e',
                    child: InkWell(
                      borderRadius: BorderRadius.circular(12),
                      onTap: () => Navigator.pop(context, e),
                      child: Container(
                        width: 44,
                        height: 44,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          color: e == widget.selected
                              ? scheme.primaryContainer
                              : null,
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Text(e, style: const TextStyle(fontSize: 24)),
                      ),
                    ),
                  ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}
