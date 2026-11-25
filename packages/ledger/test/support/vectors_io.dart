import 'dart:io';

/// The text of spec/vectors.json; tests run from packages/ledger.
String readVectors() => File('../../spec/vectors.json').readAsStringSync();
