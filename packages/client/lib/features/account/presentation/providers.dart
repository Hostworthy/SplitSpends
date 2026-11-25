import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/account/data/providers.dart';
import 'package:splitspends/features/account/domain/api_token.dart';

final tokensProvider = FutureProvider<List<ApiToken>>(
  (ref) => ref.watch(tokensRepositoryProvider).list(),
);

final profileEditsProvider = Provider<ProfileEdits>(ProfileEdits.new);

/// Saves profile changes. Each change shows at once and builds on the
/// changes before it; requests go to the server one at a time, and each
/// carries the notification settings as they are when it is sent, so quick
/// successive taps never undo each other.
class ProfileEdits {
  ProfileEdits(this._ref);

  final Ref _ref;
  Future<void> _queue = Future.value();
  var _waiting = 0;

  /// The server's copy of the profile, restored when the last request in
  /// flight fails.
  Me? _saved;

  Future<void> save({
    String? name,
    String? defaultCurrency,
    String? avatarColor,
    bool? gravatar,
    NotificationPrefs Function(NotificationPrefs prefs)? notifications,
  }) async {
    final session = _ref.read(sessionProvider.notifier);
    final me = _ref.read(meProvider);
    if (me == null) return;
    final edited = me.copyWith(
      name: name,
      defaultCurrency: defaultCurrency,
      avatarColor: avatarColor,
      gravatar: gravatar,
      notifications: notifications?.call(me.notifications),
    );
    if (_ref.read(sessionProvider).value?.isLocal ?? false) {
      final saved = await _ref
          .read(accountRepositoryProvider)
          .update(
            name: name,
            defaultCurrency: defaultCurrency,
            avatarColor: avatarColor,
            gravatar: gravatar,
            notifications: edited.notifications,
          );
      await session.updateMe(saved);
      return;
    }
    if (_waiting == 0) _saved = me;
    _waiting++;
    final previous = _queue;
    final done = Completer<void>();
    _queue = done.future;
    try {
      await session.updateMe(edited);
      await previous;
      final saved = await _ref
          .read(accountRepositoryProvider)
          .update(
            name: name,
            defaultCurrency: defaultCurrency,
            avatarColor: avatarColor,
            gravatar: gravatar,
            notifications: notifications == null
                ? null
                : (_ref.read(meProvider) ?? edited).notifications,
          );
      _saved = saved;
      if (_waiting == 1) await session.updateMe(saved);
    } catch (_) {
      if (_waiting == 1 && _saved != null) await session.updateMe(_saved!);
      rethrow;
    } finally {
      _waiting--;
      done.complete();
    }
  }
}
