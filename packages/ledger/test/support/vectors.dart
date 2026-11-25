// Reads spec/vectors.json on the Dart VM and, for the web build's number
// semantics, under Node.js, where dart:io is not available.
export 'vectors_io.dart' if (dart.library.js_interop) 'vectors_node.dart';
