import 'dart:developer' as developer;

/// Records an event under a dotted key (`area.thing.outcome`) with its
/// details, so failures the app handles still leave a trace in debug tools.
void log(
  String key, {
  Map<String, Object?> details = const {},
  Object? error,
  StackTrace? stack,
}) {
  final text = details.isEmpty
      ? key
      : '$key ${details.entries.map((e) => '${e.key}=${e.value}').join(' ')}';
  developer.log(text, name: 'splitspends', error: error, stackTrace: stack);
}
