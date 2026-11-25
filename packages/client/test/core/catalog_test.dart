// The bundled catalog must match the server's, which spec/vectors.json
// records and the Go tests in packages/core check.
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

import 'package:splitspends/core/config/app_config.dart';
import 'package:splitspends/core/config/local_config.dart';

void main() {
  final doc = jsonDecode(
    File('../../spec/vectors.json').readAsStringSync(),
  ) as Map<String, dynamic>;

  test('currencies match the server catalog', () {
    expect([
      for (final c in localConfig.currencies)
        {
          'code': c.code,
          'name': c.name,
          'symbol': c.symbol,
          'exponent': c.exponent,
        },
    ], doc['currencies']);
  });

  test('categories match the server catalog', () {
    expect([
      for (final c in localConfig.categories)
        {'id': c.id, 'name': c.name, 'group': c.group, 'icon': c.icon},
    ], doc['categories']);
  });

  test('before the server config loads, currencies use the catalog', () {
    expect(const AppConfig().currency('JPY').exponent, 0);
    expect(const AppConfig().currency('KWD').exponent, 3);
    expect(const AppConfig().currency('EUR').symbol, '€');
    expect(const AppConfig().currency('XYZ').exponent, 2);
  });
}
