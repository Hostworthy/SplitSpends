{{flutter_js}}
{{flutter_build_config}}

_flutter.loader.load({
  serviceWorkerSettings: {
    serviceWorkerVersion: {{flutter_service_worker_version}},
  },
  config: {
    // Emoji and non-Latin fallback fonts are committed in web/fonts/fallback
    // and served by our own server, never by Google Fonts. Relative, so it
    // resolves against <base href> and works under a sub-path.
    fontFallbackBaseUrl: "fonts/fallback/",
  },
});
