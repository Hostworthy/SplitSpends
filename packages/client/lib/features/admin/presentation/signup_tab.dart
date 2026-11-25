import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/admin/domain/admin.dart';
import 'package:splitspends/features/admin/presentation/providers.dart';

/// Who may sign up: the mode and the allowed list.
class SignupTab extends ConsumerStatefulWidget {
  const SignupTab({super.key});

  @override
  ConsumerState<SignupTab> createState() => _SignupTabState();
}

class _SignupTabState extends ConsumerState<SignupTab> {
  final _entry = TextEditingController();
  SignupMode? _mode;
  List<String>? _allowed;
  bool _saving = false;

  @override
  void dispose() {
    _entry.dispose();
    super.dispose();
  }

  void _add() {
    final value = _entry.text.trim().toLowerCase();
    if (value.isEmpty) return;
    setState(() {
      _allowed = {...?_allowed, value}.toList();
      _entry.clear();
    });
  }

  Future<void> _save(SignupSettings current) async {
    setState(() => _saving = true);
    try {
      final saved = await ref
          .read(adminRepositoryProvider)
          .saveSignup(
            SignupSettings(
              mode: _mode ?? current.mode,
              allowed: _allowed ?? current.allowed,
              unknownMode: _mode == null ? current.unknownMode : null,
            ),
          );
      // Switching tabs disposes this one while the save is on its way.
      if (!mounted) return;
      ref.invalidate(signupSettingsProvider);
      setState(() {
        _mode = saved.mode;
        _allowed = saved.allowed;
      });
      showMessage(context, 'Sign-up settings saved');
    } catch (error) {
      if (mounted) showError(context, error);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final settings = ref.watch(signupSettingsProvider);
    final text = Theme.of(context).textTheme;
    return settings.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (error, _) => ErrorView(
        error: error,
        onRetry: () => ref.invalidate(signupSettingsProvider),
      ),
      data: (current) {
        final mode = _mode ?? current.mode;
        final allowed = _allowed ?? current.allowed;
        final changed =
            mode != current.mode ||
            allowed.join(',') != current.allowed.join(',');
        return ListView(
          padding: const EdgeInsets.only(bottom: 48),
          children: [
            MaxWidth(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const SectionHeader('Who can sign up'),
                  if (mode == null)
                    Padding(
                      padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                      child: Text(
                        'This server uses a sign-up mode this app does not '
                        'know (${current.unknownMode ?? 'none'}). Saving the '
                        'allowed list keeps it; choose a mode below to '
                        'replace it, or update the app.',
                        style: text.bodyMedium,
                      ),
                    ),
                  for (final m in SignupMode.values)
                    RadioListTile<SignupMode>(
                      key: Key('mode-${m.name}'),
                      value: m,
                      // ignore: deprecated_member_use
                      groupValue: mode,
                      // ignore: deprecated_member_use
                      onChanged: (v) => setState(() => _mode = v),
                      title: Text(m.label),
                      subtitle: Text(m.description),
                    ),
                  const SectionHeader('Allowed list'),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    child: Text(
                      'Email addresses, or whole domains written like '
                      '@family.com. In invite and approval modes they sign '
                      'up without waiting. Closed mode ignores the list.',
                      style: text.bodySmall,
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
                    child: TextField(
                      key: const Key('allowed-entry'),
                      controller: _entry,
                      keyboardType: TextInputType.emailAddress,
                      onSubmitted: (_) => _add(),
                      decoration: InputDecoration(
                        hintText: 'name@example.com or @example.com',
                        suffixIcon: IconButton(
                          tooltip: 'Add to the list',
                          icon: const Icon(Icons.add_rounded),
                          onPressed: _add,
                        ),
                      ),
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    child: Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        if (allowed.isEmpty)
                          Text('The list is empty.', style: text.bodyMedium),
                        for (final entry in allowed)
                          InputChip(
                            label: Text(entry),
                            onDeleted: () => setState(
                              () => _allowed = [...allowed]..remove(entry),
                            ),
                          ),
                      ],
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.all(16),
                    child: FilledButton(
                      key: const Key('save-signup'),
                      onPressed: changed && !_saving
                          ? () => _save(current)
                          : null,
                      child: const Text('Save'),
                    ),
                  ),
                ],
              ),
            ),
          ],
        );
      },
    );
  }
}
