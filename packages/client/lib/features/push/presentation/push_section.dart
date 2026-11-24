import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/push/domain/push_device.dart';
import 'package:splitspends/features/push/presentation/providers.dart';
import 'package:splitspends/features/push/presentation/push_view.dart';

/// Account section: turn push on for this device, see and remove devices,
/// send a test.
class PushSection extends ConsumerStatefulWidget {
  const PushSection({super.key});

  @override
  ConsumerState<PushSection> createState() => _PushSectionState();
}

class _PushSectionState extends ConsumerState<PushSection> {
  bool _busy = false;

  String _describe(Object error) =>
      error is PushError ? error.message : describeError(error);

  Future<void> _run(Future<void> Function() action) async {
    setState(() => _busy = true);
    try {
      await action();
    } on PushError catch (error) {
      if (!mounted) return;
      if (error.needsDistributor) {
        await _explainDistributors();
      } else {
        showMessage(context, error.message);
      }
    } catch (error) {
      if (mounted) showMessage(context, _describe(error));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _enable() => _run(() async {
    final platform = ref.read(pushPlatformProvider);
    Distributor? via;
    if (platform.usesDistributors) {
      final options = await platform.distributors();
      if (options.isEmpty) {
        throw const PushError(noDistributorMessage, needsDistributor: true);
      }
      via = options.length == 1 ? options.single : await _pick(options);
      if (via == null) return;
    }
    await ref.read(pushActionsProvider).enable(via: via);
    if (mounted) showMessage(context, 'Push notifications are on.');
  });

  Future<Distributor?> _pick(List<Distributor> options) =>
      showDialog<Distributor>(
        context: context,
        builder: (context) => SimpleDialog(
          title: const Text('Deliver notifications through'),
          children: [
            for (final d in options)
              SimpleDialogOption(
                onPressed: () => Navigator.pop(context, d),
                child: Padding(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  child: Text(d.name),
                ),
              ),
          ],
        ),
      );

  Future<void> _explainDistributors() => showDialog<void>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('Install a distributor'),
      content: const Text(noDistributorMessage),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Close'),
        ),
        FilledButton(
          onPressed: () {
            Navigator.pop(context);
            launchUrl(
              Uri.parse(distributorsUrl),
              mode: LaunchMode.externalApplication,
            );
          },
          child: const Text('Find a distributor'),
        ),
      ],
    ),
  );

  Future<void> _disable() => _run(() async {
    await ref.read(pushActionsProvider).disable();
    if (mounted) showMessage(context, 'Push notifications are off here.');
  });

  Future<void> _remove(PushDevice device, bool here) =>
      _run(() => ref.read(pushActionsProvider).remove(device, here: here));

  Future<void> _test() => _run(() async {
    final result = await ref.read(pushActionsProvider).test();
    if (mounted) showMessage(context, result.summary);
  });

  @override
  Widget build(BuildContext context) {
    final async = ref.watch(pushViewProvider);
    final view = async.value;
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Push notifications'),
        if (view == null)
          async.hasError
              ? ListTile(
                  leading: const Icon(Icons.error_outline_rounded),
                  title: Text(_describe(async.error!)),
                  trailing: TextButton(
                    onPressed: () => ref.invalidate(pushDevicesProvider),
                    child: const Text('Retry'),
                  ),
                )
              : const ListTile(
                  leading: SizedBox.square(
                    dimension: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                  title: Text('Checking this device'),
                )
        else ...[
          ListTile(
            leading: Icon(
              view.state == PushState.on
                  ? Icons.notifications_active_rounded
                  : Icons.notifications_off_outlined,
            ),
            title: Text(view.message),
          ),
          if (view.canEnable || view.canDisable || view.canTest)
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  if (view.canEnable)
                    FilledButton.icon(
                      onPressed: _busy ? null : _enable,
                      icon: const Icon(Icons.notifications_rounded),
                      label: const Text('Enable push notifications'),
                    ),
                  if (view.canTest)
                    OutlinedButton.icon(
                      onPressed: _busy ? null : _test,
                      icon: const Icon(Icons.send_rounded),
                      label: const Text('Send test'),
                    ),
                  if (view.canDisable)
                    TextButton(
                      onPressed: _busy ? null : _disable,
                      child: const Text('Turn off on this device'),
                    ),
                ],
              ),
            ),
          for (final device in view.devices)
            ListTile(
              leading: Icon(
                device.platform == 'unifiedpush'
                    ? Icons.phone_android_rounded
                    : Icons.public_rounded,
              ),
              title: Text(
                device.id == view.thisDevice
                    ? '${device.displayName} (this device)'
                    : device.displayName,
              ),
              subtitle: Text(
                _deviceLine(device),
                style: device.failures > 0
                    ? TextStyle(color: theme.colorScheme.error)
                    : null,
              ),
              trailing: IconButton(
                tooltip: 'Remove device',
                icon: const Icon(Icons.delete_outline_rounded),
                onPressed: _busy
                    ? null
                    : () => _remove(device, device.id == view.thisDevice),
              ),
            ),
        ],
      ],
    );
  }
}

String _deviceLine(PushDevice device) {
  String day(String raw) {
    final at = DateTime.tryParse(raw);
    return at == null ? raw : DateFormat.yMMMd().format(at.toLocal());
  }

  final added = 'Added ${day(device.createdAt)}';
  if (device.failures > 0) {
    return '$added · last ${device.failures} '
        'deliver${device.failures == 1 ? 'y' : 'ies'} failed';
  }
  final last = device.lastSuccessAt;
  return last == null ? added : '$added · last delivered ${day(last)}';
}
