# ADR-003: Email One-Time Codes, Sessions and API Tokens

## Status

Accepted

## Context

Splitting expenses needs identity but not passwords. Everyone already has email, and people are often added by email before they sign up.

## Decision

- Login is a 6-digit code sent by email. Codes are random, valid 10 minutes, open one session, and are stored as an HMAC bound to the challenge id and email with the server `SECRET_KEY`, so a database leak cannot be brute-forced offline.
- Each code is compared at most 5 times. Verifying counts one attempt against every live code of the address in a single database statement before any code is compared, so concurrent requests cannot add guesses, and a code is marked used with a conditional update, so it opens one session however many requests carry it. Because every live code is checked, codes someone else requests do not hide the one the owner holds.
- Requesting a code for an email that may not sign up returns the same response and sends nothing, so addresses cannot be probed: the request is recorded without a code and counts toward the same limits, and the code email is queued rather than sent while the caller waits, so neither a slow nor a failing mail server shows in the response (failures are logged). With `SERVERLESS=true` emails are sent before the response, so there the time taken still differs. Codes are limited per hour to 5 per email from one network (an IPv4 address or an IPv6 /64, stored only as a keyed hash) and 10 per email in total, counted under a per-email lock. Someone who knows an address can use up only their own network's share, so its owner can still get a code; the total bounds the guesses per address at 50 an hour. Code endpoints are also rate limited per network.
- Someone who keeps sending wrong codes for an address can still use up the attempts of every live code, including the owner's, and an attacker on several networks can use up the per-email total; both need sustained traffic and are slowed by the per-network rate limit. Telling the owner apart from them would need codes bound to the network that requested them, which breaks sign-in when a phone changes networks.
- The sign-up rules are applied when a code is requested and again when it is verified, inside the transaction that opens the session, so a code sent before an account was disabled, sign-up was closed or an invite link was revoked does not let anyone in. A code requested through an invite link remembers the link by its storage hash and checks it again. A disabled account becomes active again only when an administrator enables it.
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
