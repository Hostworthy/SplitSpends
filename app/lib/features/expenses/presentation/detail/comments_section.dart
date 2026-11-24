import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/avatar.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/expenses/presentation/providers.dart';

class CommentsSection extends ConsumerStatefulWidget {
  const CommentsSection({super.key, required this.expenseId});

  final String expenseId;

  @override
  ConsumerState<CommentsSection> createState() => _CommentsSectionState();
}

class _CommentsSectionState extends ConsumerState<CommentsSection> {
  final _controller = TextEditingController();
  bool _sending = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final body = _controller.text.trim();
    if (body.isEmpty || _sending) return;
    setState(() => _sending = true);
    try {
      await ref
          .read(expensesRepositoryProvider)
          .addComment(widget.expenseId, body);
      _controller.clear();
      ref.invalidate(commentsProvider(widget.expenseId));
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  Future<void> _delete(String commentId) async {
    try {
      await ref.read(expensesRepositoryProvider).deleteComment(commentId);
      ref.invalidate(commentsProvider(widget.expenseId));
    } catch (error) {
      if (mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context) {
    final comments = ref.watch(commentsProvider(widget.expenseId));
    final meId = ref.watch(meProvider)?.id;
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Comments'),
        ...switch (comments) {
          AsyncValue(:final value?) => [
            if (value.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: Text('No comments yet.', style: TextStyle(color: muted)),
              ),
            for (final c in value)
              ListTile(
                leading: UserAvatar(user: c.user, size: 34),
                title: Text(c.body),
                subtitle: Text(
                  '${c.user.id == meId ? 'You' : c.user.displayName} · '
                  '${Dates.relative(Dates.tryParse(c.createdAt) ?? DateTime.now())}',
                ),
                trailing: c.user.id == meId
                    ? IconButton(
                        tooltip: 'Delete comment',
                        icon: const Icon(Icons.close_rounded, size: 18),
                        onPressed: () => _delete(c.id),
                      )
                    : null,
              ),
          ],
          AsyncValue(:final error?) => [ErrorView(error: error)],
          _ => [const LinearProgressIndicator()],
        },
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
          child: TextField(
            controller: _controller,
            minLines: 1,
            maxLines: 4,
            textInputAction: TextInputAction.send,
            onSubmitted: (_) => _send(),
            decoration: InputDecoration(
              hintText: 'Add a comment',
              suffixIcon: IconButton(
                icon: _sending
                    ? const SizedBox.square(
                        dimension: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.send_rounded),
                onPressed: _send,
              ),
            ),
          ),
        ),
      ],
    );
  }
}
