import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Whether the last server request got through. Starts optimistic.
final reachabilityProvider = NotifierProvider<Reachability, bool>(
  Reachability.new,
);

class Reachability extends Notifier<bool> {
  @override
  bool build() => true;

  void report(bool reachable) {
    if (state != reachable) state = reachable;
  }
}
