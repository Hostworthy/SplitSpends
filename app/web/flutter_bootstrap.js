{{flutter_js}}
{{flutter_build_config}}

_flutter.loader.load({
  serviceWorkerSettings: {
    serviceWorkerVersion: {{flutter_service_worker_version}},
  },
  config: {
    // Emoji and non-Latin fallback fonts come from this server, not
    // fonts.gstatic.com. tool/fallback_fonts.sh downloads them. Relative, so
    // it resolves against <base href> and works under a sub-path.
    fontFallbackBaseUrl: "fonts/fallback/",
  },
});
