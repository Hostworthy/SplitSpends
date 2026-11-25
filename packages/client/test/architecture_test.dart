// Enforces the client's layers, like packages/archtest does for Go:
// core <- features <- app. core is the foundation and knows no feature,
// features never reach into the app shell, a feature's domain is plain
// Dart, and a feature uses another feature only through its domain, its
// providers and its widgets, never through its data implementations.
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

const _package = 'package:splitspends/';

/// Packages that tie code to Flutter or to Riverpod's widget bindings.
const _ui = [
  'package:flutter/',
  'package:flutter_riverpod/',
  'package:material_ui/',
  'package:cupertino_ui/',
];

/// Every Dart file under lib/, as a path relative to lib/ like
/// `features/groups/domain/group.dart`, with the URIs it imports or exports.
Map<String, List<String>> _load() {
  final directive = RegExp(r'^(?:import|export)\b[^;]*;', multiLine: true);
  final uri = RegExp(r"'([^']+)'");
  return {
    for (final file in Directory('lib').listSync(recursive: true))
      if (file is File && file.path.endsWith('.dart'))
        _relative(file.path): [
          for (final d in directive.allMatches(file.readAsStringSync()))
            for (final u in uri.allMatches(d.group(0)!)) u.group(1)!,
        ],
  };
}

String _relative(String path) =>
    path.replaceAll(r'\', '/').replaceFirst(RegExp('^lib/'), '');

/// The lib/-relative path [target] names when imported from [from], or null
/// for another package or a dart: library.
String? _resolve(String from, String target) {
  if (target.startsWith(_package)) return target.substring(_package.length);
  if (target.contains(':')) return null;
  final parts = from.split('/')..removeLast();
  for (final part in target.split('/')) {
    if (part == '..') {
      parts.removeLast();
    } else if (part != '.') {
      parts.add(part);
    }
  }
  return parts.join('/');
}

/// `groups` for `features/groups/...`, otherwise null.
String? _feature(String path) {
  final parts = path.split('/');
  return parts.length > 2 && parts.first == 'features' ? parts[1] : null;
}

/// The layer inside a feature: domain, data, presentation or platform.
String? _layer(String path) {
  final parts = path.split('/');
  return parts.length > 3 && parts.first == 'features' ? parts[2] : null;
}

void main() {
  final files = _load();

  /// Every import edge from a file matching [from] as "file -> target",
  /// where [bad] says the resolved target (or the raw URI for another
  /// package) breaks the rule.
  List<String> violations(
    bool Function(String file) from,
    bool Function(String file, String target) bad,
  ) => [
    for (final MapEntry(key: file, value: targets) in files.entries)
      if (from(file))
        for (final target in targets)
          if (bad(file, _resolve(file, target) ?? target)) '$file -> $target',
  ];

  test('the layout is the one the rules describe', () {
    expect(files.keys, contains('app/router.dart'));
    expect(files.keys.where((f) => f.startsWith('core/')), isNotEmpty);
    expect(files.keys.where((f) => _layer(f) == 'domain'), isNotEmpty);
  });

  test('core imports neither the app nor any feature', () {
    expect(
      violations(
        (file) => file.startsWith('core/'),
        (_, target) =>
            target.startsWith('app/') || target.startsWith('features/'),
      ),
      isEmpty,
    );
  });

  test('features never import the app shell', () {
    expect(
      violations(
        (file) => file.startsWith('features/'),
        (_, target) => target.startsWith('app/'),
      ),
      isEmpty,
    );
  });

  test('only the app imports the router', () {
    expect(
      violations(
        (file) => !file.startsWith('app/'),
        (_, target) => target == 'app/router.dart',
      ),
      isEmpty,
    );
  });

  test('a domain imports no data, presentation or platform code', () {
    expect(
      violations(
        (file) => _layer(file) == 'domain',
        (_, target) =>
            const ['data', 'presentation', 'platform'].contains(_layer(target)),
      ),
      isEmpty,
    );
  });

  test('a domain is plain Dart, even through what it imports', () {
    final reached = <String>[];
    for (final start in files.keys.where((f) => _layer(f) == 'domain')) {
      final seen = <String>{};
      final pending = [start];
      while (pending.isNotEmpty) {
        final file = pending.removeLast();
        if (!seen.add(file)) continue;
        for (final target in files[file] ?? const <String>[]) {
          if (_ui.any(target.startsWith)) {
            reached.add('$start reaches $target through $file');
          }
          final next = _resolve(file, target);
          if (next != null) pending.add(next);
        }
      }
    }
    expect(reached, isEmpty);
  });

  test('a feature uses another feature only through its public parts', () {
    expect(
      violations((file) => _feature(file) != null, (file, target) {
        final other = _feature(target);
        if (other == null || other == _feature(file)) return false;
        return switch (_layer(target)) {
          'domain' || 'presentation' => false,
          'data' => !target.endsWith('/data/providers.dart'),
          _ => true,
        };
      }),
      isEmpty,
    );
  });
}
