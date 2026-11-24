import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/utils/dates.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/features/account/presentation/providers.dart';

/// Personal API tokens for the MCP server and other clients.
class TokensScreen extends ConsumerWidget {
  const TokensScreen({super.key});

  Future<void> _create(BuildContext context, WidgetRef ref) async {
    final controller = TextEditingController(text: 'MCP');
    final name = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('New API token'),
        content: TextField(
          controller: controller,
          autofocus: true,
          decoration: const InputDecoration(
            labelText: 'Name',
            hintText: 'Claude Desktop, laptop…',
          ),
          onSubmitted: (v) => Navigator.pop(context, v),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, controller.text),
            child: const Text('Create'),
          ),
        ],
      ),
    );
    controller.dispose();
    if (name == null || name.trim().isEmpty) return;
    try {
      final token = await ref
          .read(tokensRepositoryProvider)
          .create(name.trim());
      ref.invalidate(tokensProvider);
      if (!context.mounted || token.token == null) return;
      await showDialog<void>(
        context: context,
        builder: (context) => _TokenCreatedDialog(token: token.token!),
      );
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  Future<void> _revoke(BuildContext context, WidgetRef ref, String id) async {
    try {
      await ref.read(tokensRepositoryProvider).revoke(id);
      ref.invalidate(tokensProvider);
    } catch (error) {
      if (context.mounted) showError(context, error);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final tokens = ref.watch(tokensProvider);
    final server = ref.watch(sessionProvider).value?.serverUrl ?? '';
    final text = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(title: const Text('API tokens')),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => _create(context, ref),
        icon: const Icon(Icons.add_rounded),
        label: const Text('New token'),
      ),
      body: ListView(
        padding: const EdgeInsets.only(bottom: 96),
        children: [
          MaxWidth(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Card(
                  margin: const EdgeInsets.all(16),
                  color: scheme.secondaryContainer,
                  child: Padding(
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Use Split Spends from AI assistants',
                          style: text.titleMedium,
                        ),
                        const SizedBox(height: 6),
                        Text(
                          'Tokens let MCP clients (like Claude) read balances and add '
                          'expenses on your behalf. Point your client at '
                          '$server/mcp and send the token as a Bearer header. '
                          'Treat tokens like passwords; revoke any you no longer use.',
                          style: text.bodyMedium,
                        ),
                      ],
                    ),
                  ),
                ),
                AsyncView(
                  value: tokens,
                  onRetry: () => ref.invalidate(tokensProvider),
                  data: (items) => items.isEmpty
                      ? const EmptyState(
                          icon: Icons.key_off_rounded,
                          title: 'No tokens yet',
                        )
                      : Column(
                          children: [
                            for (final t in items)
                              ListTile(
                                leading: const Icon(Icons.key_rounded),
                                title: Text(t.name),
                                subtitle: Text(
                                  'Created ${Dates.relative(Dates.tryParse(t.createdAt) ?? DateTime.now())}'
                                  '${t.lastUsedAt == null ? ' · never used' : ' · used ${Dates.relative(Dates.tryParse(t.lastUsedAt) ?? DateTime.now())}'}',
                                ),
                                trailing: TextButton(
                                  onPressed: () => _revoke(context, ref, t.id),
                                  child: const Text('Revoke'),
                                ),
                              ),
                          ],
                        ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _TokenCreatedDialog extends StatelessWidget {
  const _TokenCreatedDialog({required this.token});

  final String token;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Copy your token'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('You will not see it again.'),
          const SizedBox(height: 12),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Theme.of(context).colorScheme.surfaceContainerHighest,
              borderRadius: BorderRadius.circular(12),
            ),
            child: SelectableText(
              token,
              style: const TextStyle(fontFamily: 'monospace'),
            ),
          ),
        ],
      ),
      actions: [
        TextButton.icon(
          onPressed: () async {
            await Clipboard.setData(ClipboardData(text: token));
            if (context.mounted) showMessage(context, 'Token copied');
          },
          icon: const Icon(Icons.copy_rounded),
          label: const Text('Copy'),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Done'),
        ),
      ],
    );
  }
}
