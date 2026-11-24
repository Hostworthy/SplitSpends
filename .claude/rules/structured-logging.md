---
paths:
  - "apps/**/*.go"
  - "packages/**/*.go"
---
# Structured Logging

- The `slog` message is a stable dotted key: `service.area.outcome`, lowercase, kebab-case inside a segment (`middleware.error.internal`).
- Put ids, paths, counts and errors in attributes, not in the message.
- Infrastructure errors (database, config, SMTP, AI) start with the same style of key: `fmt.Errorf("database.ping.failed: %w", err)`.
- Domain errors (`errs.Error`) keep human-readable messages because clients show them.
- Never log secrets, tokens, login codes (except the development log mailer) or receipt contents.
