# ADR-003: Email One-Time Codes, Sessions and API Tokens

## Status

Accepted

## Context

Splitting expenses needs identity but not passwords. Everyone already has email, and people are often added by email before they sign up.

## Decision

- Login is a 6-digit code sent by email. Codes are random, valid 10 minutes, allow 5 attempts, and are stored as an HMAC bound to the challenge id and email with the server `SECRET_KEY`, so a database leak cannot be brute-forced offline.
- Requesting a code for an email that may not sign up returns the same response and sends nothing, so addresses cannot be probed. Codes per email are limited to 5 per hour; code endpoints are also rate limited per IP.
- `SIGNUP_MODE` decides who may create an account: `open`, `invite` (default: the first account plus anyone an existing user added) or `domain`.
- A successful login creates an opaque random bearer token; only its SHA-256 hash is stored. Sessions expire after `SESSION_TTL_DAYS`.
- Personal API tokens (`ss_` prefix) use the same table with kind `token`, never expire, and can be revoked. They exist for the MCP server and scripts.
- Invited users get a real user row immediately so expenses can reference them; their first login activates the account and keeps all history.

## Consequences

### Positive

- No passwords to store, reset or leak.
- Mobile and web share one bearer-token scheme; no cookies means no CSRF surface.

### Negative

- Login depends on working email delivery. In development, codes are printed to the log.
