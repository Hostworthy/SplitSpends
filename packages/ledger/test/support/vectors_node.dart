import 'dart:js_interop';

@JS('require')
external _Fs _require(String id);

extension type _Fs(JSObject _) implements JSObject {
  external String readFileSync(String path, String encoding);
}

/// The text of spec/vectors.json; tests run from packages/ledger.
String readVectors() =>
    _require('fs').readFileSync('../../spec/vectors.json', 'utf8');
