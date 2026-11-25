import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:sembast/sembast_memory.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/app/repositories.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/config_provider.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/features/push/domain/push_device.dart';
import 'package:splitspends/features/push/domain/push_repository.dart';
import 'package:splitspends/features/push/platform/push_platform.dart';
import 'package:splitspends/features/push/presentation/providers.dart';
import 'package:splitspends/features/push/presentation/push_section.dart';
import 'package:splitspends/features/push/presentation/push_view.dart';

import '../helpers.dart';

const _keys = PushKeys(
  endpoint: 'https://push.example.com/abc',
  p256dh: 'BPk',
  auth: 'au',
  platform: 'web',
  label: 'Firefox on Linux',
);

PushDevice _device(String id, String endpoint, {int failures = 0}) =>
    PushDevice(
      id: id,
      platform: 'web',
      label: 'Firefox on Linux',
      endpoint: endpoint,
      createdAt: '2026-11-20T10:00:00Z',
      failures: failures,
    );

class _FakePlatform implements PushPlatform {
  _FakePlatform({this.distributorList, this.reason, this.subscribed});

  final List<Distributor>? distributorList;
  final String? reason;
  PushKeys? subscribed;
  String? vapidKey;
  Distributor? via;

  @override
  String? get unsupportedReason => reason;

  @override
  bool get usesDistributors => distributorList != null;

  @override
  Future<List<Distributor>> distributors() async => distributorList ?? [];

  @override
  Future<PushKeys?> current() async => subscribed;

  @override
  Future<PushKeys> subscribe({required String vapidKey, Distributor? via}) {
    this.vapidKey = vapidKey;
    this.via = via;
    subscribed = _keys;
    return Future.value(_keys);
  }

  @override
  Future<void> unsubscribe() async => subscribed = null;

  @override
  void watch(void Function(PushKeys keys) onChange) {}
}

class _FakeRepository implements PushRepository {
  _FakeRepository({this.failure});

  /// Thrown by every call when set, as when the server is unreachable.
  final ApiError? failure;
  final devices = <PushDevice>[];
  var tests = 0;

  @override
  Future<List<PushDevice>> list() async {
    if (failure case final failure?) throw failure;
    return List.of(devices);
  }

  @override
  Future<PushDevice> register(PushKeys keys) async {
    final d = _device('d${devices.length + 1}', keys.endpoint);
    devices.add(d);
    return d;
  }

  @override
  Future<void> remove(String id) async =>
      devices.removeWhere((d) => d.id == id);

  @override
  Future<PushTestResult> test() async {
    tests++;
    return PushTestResult(sent: devices.length);
  }
}

class _ConnectedSession extends SessionController {
  @override
  Future<Session> build() async => const Session(
    mode: AppMode.connected,
    serverUrl: 'https://split.example.com',
    token: 'token',
    me: Me(id: 'u1', name: 'Ada'),
  );
}

Future<void> _pump(
  WidgetTester tester,
  _FakePlatform platform,
  _FakeRepository repo,
) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        memoryDatabase(),
        sessionProvider.overrideWith(_ConnectedSession.new),
        configProvider.overrideWith(
          (ref) async =>
              const AppConfig(pushEnabled: true, vapidPublicKey: 'server-key'),
        ),
        pushPlatformProvider.overrideWithValue(platform),
        pushRepositoryProvider.overrideWithValue(repo),
      ],
      child: const MaterialApp(
        home: Scaffold(body: SingleChildScrollView(child: PushSection())),
      ),
    ),
  );
  await settle(tester);
}

void main() {
  group('PushView', () {
    test('on-device mode explains that push needs a server', () {
      final v = PushView.resolve(connected: false, serverEnabled: true);
      expect(v.state, PushState.needsServer);
      expect(v.message, contains('need a server'));
      expect(v.canEnable || v.canTest || v.showSwitches, isFalse);
    });

    test('a server without push says so', () {
      final v = PushView.resolve(connected: true, serverEnabled: false);
      expect(v.state, PushState.serverOff);
      expect(v.canEnable, isFalse);
    });

    test('unsupported platforms show their reason but list devices', () {
      final v = PushView.resolve(
        connected: true,
        serverEnabled: true,
        unsupportedReason:
            'Push notifications on iPhone are not available yet.',
        devices: [_device('d1', 'https://push.example.com/other')],
      );
      expect(v.state, PushState.unsupported);
      expect(v.message, 'Push notifications on iPhone are not available yet.');
      expect(v.canEnable, isFalse);
      expect(v.canTest, isTrue, reason: 'other devices can still be tested');
    });

    test('off until this device is registered on the server', () {
      final off = PushView.resolve(
        connected: true,
        serverEnabled: true,
        current: _keys,
      );
      expect(off.state, PushState.off);
      expect(off.canEnable, isTrue);
      expect(off.canTest, isFalse);

      final on = PushView.resolve(
        connected: true,
        serverEnabled: true,
        current: _keys,
        devices: [
          _device('d1', 'https://push.example.com/other'),
          _device('d2', _keys.endpoint),
        ],
      );
      expect(on.state, PushState.on);
      expect(on.thisDevice, 'd2');
      expect(on.canDisable && on.canTest && !on.canEnable, isTrue);
    });
  });

  group('preferences', () {
    test('email keys stay top level and push is nested', () {
      const prefs = NotificationPrefs();
      final next = prefs.copyWith(
        email: prefs.email.set(NotificationKind.expenseAdded, false),
        push: prefs.push.set(NotificationKind.paymentReceived, false),
      );
      final json = next.toJson();
      expect(json['expenseAdded'], isFalse);
      expect(json['paymentReceived'], isTrue);
      expect((json['push'] as Map)['paymentReceived'], isFalse);
      expect((json['push'] as Map)['expenseAdded'], isTrue);
      final back = NotificationPrefs.fromJson(json);
      expect(back.email.on(NotificationKind.expenseAdded), isFalse);
      expect(back.push.on(NotificationKind.paymentReceived), isFalse);
    });

    test('approvals round-trip and default on', () {
      final prefs = NotificationPrefs.fromJson({'approvals': false});
      expect(prefs.email.on(NotificationKind.approvals), isFalse);
      expect(prefs.push.on(NotificationKind.approvals), isTrue);
      final json = prefs
          .copyWith(push: prefs.push.set(NotificationKind.approvals, false))
          .toJson();
      expect(json['approvals'], isFalse);
      expect((json['push'] as Map)['approvals'], isFalse);
      expect(const ChannelPrefs().on(NotificationKind.approvals), isTrue);
    });

    test('only administrators and managers see the approvals switch', () {
      Me withRole(String role) => Me(id: 'u1', name: 'Ada', role: role);
      expect(
        withRole('user').notificationKinds,
        isNot(contains(NotificationKind.approvals)),
      );
      expect(
        withRole('user').notificationKinds,
        contains(NotificationKind.reminders),
      );
      for (final role in ['manager', 'admin']) {
        expect(withRole(role).notificationKinds, NotificationKind.values);
      }
    });

    test('profiles saved before push default every push switch on', () {
      final prefs = NotificationPrefs.fromJson({'expenseAdded': false});
      expect(prefs.email.on(NotificationKind.expenseAdded), isFalse);
      for (final k in NotificationKind.values) {
        expect(prefs.push.on(k), isTrue);
      }
    });
  });

  test('names distributors and browsers', () {
    expect(distributorName('io.heckel.ntfy'), 'ntfy');
    expect(distributorName('org.example.mypush'), 'Mypush');
    expect(
      browserLabel(
        'Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0',
      ),
      'Firefox on Linux',
    );
    expect(
      browserLabel(
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 '
        '(KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140.0',
      ),
      'Edge on Windows',
    );
    expect(
      const PushTestResult(sent: 2, failed: 1).summary,
      'Sent to 2 devices, 1 failed.',
    );
  });

  testWidgets('enabling registers this device and allows a test', (
    tester,
  ) async {
    final platform = _FakePlatform();
    final repo = _FakeRepository();
    await _pump(tester, platform, repo);

    expect(find.text('Enable push notifications'), findsOneWidget);
    expect(find.text('Send test'), findsNothing);

    await tester.tap(find.text('Enable push notifications'));
    await settle(tester);

    expect(platform.vapidKey, 'server-key');
    expect(repo.devices.single.endpoint, _keys.endpoint);
    expect(find.text('Firefox on Linux (this device)'), findsOneWidget);
    expect(find.text('Enable push notifications'), findsNothing);

    await tester.tap(find.text('Send test'));
    await settle(tester);
    expect(repo.tests, 1);
    expect(find.text('Sent to 1 device.'), findsOneWidget);

    await tester.tap(find.text('Turn off on this device'));
    await settle(tester);
    expect(repo.devices, isEmpty);
    expect(platform.subscribed, isNull);
    expect(find.text('Enable push notifications'), findsOneWidget);
  });

  test(
    'disable unsubscribes locally even when the server list fails',
    () async {
      final platform = _FakePlatform(subscribed: _keys);
      final container = ProviderContainer(
        overrides: [
          sessionProvider.overrideWith(_ConnectedSession.new),
          pushPlatformProvider.overrideWithValue(platform),
          pushRepositoryProvider.overrideWithValue(
            _FakeRepository(
              failure: const ApiError.network('Cannot reach the server.'),
            ),
          ),
        ],
      );
      addTearDown(container.dispose);

      await expectLater(
        container.read(pushActionsProvider).disable(),
        throwsA(isA<ApiError>()),
      );
      expect(platform.subscribed, isNull);

      platform.subscribed = _keys;
      await container.read(pushActionsProvider).forget();
      expect(platform.subscribed, isNull);
    },
  );

  group('PushActions', () {
    ProviderContainer actions(_FakePlatform platform, _FakeRepository repo) {
      final container = ProviderContainer(
        overrides: [
          sessionProvider.overrideWith(_ConnectedSession.new),
          pushPlatformProvider.overrideWithValue(platform),
          pushRepositoryProvider.overrideWithValue(repo),
        ],
      );
      addTearDown(container.dispose);
      return container;
    }

    test('disable removes only this device and unsubscribes', () async {
      final platform = _FakePlatform(subscribed: _keys);
      final repo = _FakeRepository()
        ..devices.addAll([
          _device('d1', 'https://push.example.com/other'),
          _device('d2', _keys.endpoint),
        ]);

      await actions(platform, repo).read(pushActionsProvider).disable();

      expect(repo.devices.map((d) => d.id), ['d1']);
      expect(platform.subscribed, isNull);
    });

    test('disable without a subscription leaves the server alone', () async {
      final platform = _FakePlatform();
      final repo = _FakeRepository()
        ..devices.add(_device('d1', 'https://push.example.com/other'));

      await actions(platform, repo).read(pushActionsProvider).disable();

      expect(repo.devices, hasLength(1));
    });

    test('removing another device keeps push on here', () async {
      final platform = _FakePlatform(subscribed: _keys);
      final other = _device('d1', 'https://push.example.com/other');
      final repo = _FakeRepository()..devices.add(other);

      await actions(
        platform,
        repo,
      ).read(pushActionsProvider).remove(other, here: false);

      expect(repo.devices, isEmpty);
      expect(platform.subscribed, isNotNull);
    });

    test('removing this device turns push off here', () async {
      final platform = _FakePlatform(subscribed: _keys);
      final mine = _device('d1', _keys.endpoint);
      final repo = _FakeRepository()..devices.add(mine);

      await actions(
        platform,
        repo,
      ).read(pushActionsProvider).remove(mine, here: true);

      expect(repo.devices, isEmpty);
      expect(platform.subscribed, isNull);
    });

    test('forget removes this device and never throws', () async {
      final platform = _FakePlatform(subscribed: _keys);
      final repo = _FakeRepository()
        ..devices.add(_device('d1', _keys.endpoint));

      await actions(platform, repo).read(pushActionsProvider).forget();

      expect(repo.devices, isEmpty);
      expect(platform.subscribed, isNull);
    });
  });

  group('start-up sync', () {
    late Database db;
    setUp(() async {
      db = await databaseFactoryMemory.openDatabase(
        'test-${DateTime.now().microsecondsSinceEpoch}',
      );
    });

    /// Starts the app's providers once, as each app start does.
    Future<void> start(_FakePlatform platform, _FakeRepository repo) async {
      final container = ProviderContainer(
        overrides: [
          databaseProvider.overrideWith((ref) async => db),
          sessionProvider.overrideWith(_ConnectedSession.new),
          pushPlatformProvider.overrideWithValue(platform),
          pushRepositoryProvider.overrideWithValue(repo),
        ],
      );
      addTearDown(container.dispose);
      await container.read(sessionProvider.future);
      await container.read(pushSyncProvider.future);
    }

    test('sends a subscription the server has not had, once', () async {
      final platform = _FakePlatform(subscribed: _keys);
      final repo = _FakeRepository();

      await start(platform, repo);
      expect(repo.devices.map((d) => d.endpoint), [_keys.endpoint]);

      await start(platform, repo);
      expect(repo.devices, hasLength(1));

      platform.subscribed = const PushKeys(
        endpoint: 'https://push.example.com/new',
        p256dh: 'BPn',
        auth: 'an',
        platform: 'web',
      );
      await start(platform, repo);
      expect(repo.devices.map((d) => d.endpoint), [
        _keys.endpoint,
        'https://push.example.com/new',
      ]);
    });

    test('a device removed elsewhere is not added back', () async {
      final platform = _FakePlatform(subscribed: _keys);
      final repo = _FakeRepository();
      await start(platform, repo);
      repo.devices.clear();

      await start(platform, repo);

      expect(repo.devices, isEmpty);
    });

    test('sends nothing without a subscription', () async {
      final repo = _FakeRepository();
      await start(_FakePlatform(), repo);
      expect(repo.devices, isEmpty);
    });
  });

  testWidgets('a 401 sign-out unsubscribes this device', (tester) async {
    tester.view.physicalSize = const Size(420, 2000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final platform = _FakePlatform(subscribed: _keys);
    final store = MemoryLocalStore(
      serverUrl: 'http://splitspends.test',
      token: 'tok',
      appMode: AppMode.connected.name,
    );
    final api = MockClient((request) async {
      final path = request.url.path;
      if (path.endsWith('/me/balances')) {
        return http.Response(
          jsonEncode({'error': 'Please sign in.', 'code': 'UNAUTHORIZED'}),
          401,
        );
      }
      if (path.endsWith('/me')) {
        return http.Response(
          jsonEncode({
            'data': {'id': 'u1', 'name': 'Ada', 'defaultCurrency': 'EUR'},
          }),
          200,
        );
      }
      final Object body = path.endsWith('/config')
          ? {
              'data': {
                'appName': 'SplitSpends',
                'currencies': <Object>[],
                'categories': <Object>[],
              },
            }
          : {'items': <Object>[], 'total': 0};
      return http.Response(jsonEncode(body), 200);
    });
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [
          localStoreProvider.overrideWithValue(store),
          memoryDatabase(),
          httpClientProvider.overrideWithValue(api),
          pushPlatformProvider.overrideWithValue(platform),
        ],
        child: const SplitSpendsApp(),
      ),
    );
    await settle(tester);

    expect(store.token, isNull);
    expect(find.text('Use on this device'), findsOneWidget);
    expect(platform.subscribed, isNull);
    expect(await pumpFuture(tester, platform.current()), isNull);
  });

  testWidgets('Android without a distributor explains what to install', (
    tester,
  ) async {
    final platform = _FakePlatform(distributorList: const []);
    await _pump(tester, platform, _FakeRepository());

    await tester.tap(find.text('Enable push notifications'));
    await settle(tester);

    expect(find.text('Install a distributor'), findsOneWidget);
    expect(find.textContaining('UnifiedPush distributor'), findsOneWidget);
    expect(find.text('Find a distributor'), findsOneWidget);
    expect(platform.subscribed, isNull);
  });

  testWidgets('Android with one distributor uses it', (tester) async {
    final platform = _FakePlatform(
      distributorList: const [Distributor('io.heckel.ntfy')],
    );
    final repo = _FakeRepository();
    await _pump(tester, platform, repo);

    await tester.tap(find.text('Enable push notifications'));
    await settle(tester);

    expect(platform.via?.name, 'ntfy');
    expect(repo.devices, hasLength(1));
  });

  testWidgets('iPhone shows that push is not available yet', (tester) async {
    final platform = _FakePlatform(
      reason: 'Push notifications on iPhone are not available yet.',
    );
    await _pump(tester, platform, _FakeRepository());
    expect(
      find.text('Push notifications on iPhone are not available yet.'),
      findsOneWidget,
    );
    expect(find.text('Enable push notifications'), findsNothing);
  });
}
