# ADR-003: Email One-Time Codes, Sessions and API Tokens

## Status

Accepted. Sign-up modes and the first account amended by [ADR-010](010-administration.md).

## Context

Splitting expenses needs identity but not passwords. Everyone already has email, and people are often added by email before they sign up.

## Decision

- Login is a 6-digit code sent by email. Codes are random, valid 10 minutes, open one session, and are stored as an HMAC bound to the challenge id and email with the server `SECRET_KEY`, so a database leak cannot be brute-forced offline.
- Each code is compared at most 5 times. Verifying counts one attempt against every live code of the address in a single database statement before any code is compared, so concurrent requests cannot add guesses, and a code is marked used with a conditional update, so it opens one session however many requests carry it. Because every live code is checked, codes someone else requests do not hide the one the owner holds.
- Requesting a code for an email that may not sign up returns the same response and sends nothing, so addresses cannot be probed: the request is recorded without a code and counts toward the same limits, and the code email is queued rather than sent while the caller waits, so neither a slow nor a failing mail server shows in the response (failures are logged). With `SERVERLESS=true` emails are sent before the response, so there the time taken still differs. Codes are limited per hour to 5 per email from one network (an IPv4 address or an IPv6 /64, stored only as a keyed hash) and 10 per email in total, counted under a per-email lock. Someone who knows an address can use up only their own network's share, so its owner can still get a code; the total bounds the guesses per address at 50 an hour. Code endpoints are also rate limited per network.
- Someone who keeps sending wrong codes for an address can still use up the attempts of every live code, including the owner's, and an attacker on several networks can use up the per-email total; both need sustained traffic and are slowed by the per-network rate limit. Telling the owner apart from them would need codes bound to the network that requested them, which breaks sign-in when a phone changes networks.
- The sign-up rules are applied when a code is requested and again when it is verified, inside the transaction that opens the session, so a code sent before an account was disabled, sign-up was closed or an invite link was revoked does not let anyone in. A code requested through an invite link remembers the link by its storage hash and checks it again. A disabled account becomes active again only when an administrator enables it.
- Who may create an account is decided by the sign-up mode, `open`, `invite`, `approval` or `closed`, which lives in the database and is changed in the app; `SIGNUP_MODE` only gives its starting value. [ADR-010](010-administration.md) records the modes. `domain`, an earlier mode, is still read from `SIGNUP_MODE` as `invite` with `SIGNUP_ALLOWED_DOMAINS` as the allowed list. The very first account can sign up in every mode and becomes the administrator; it is decided under a lock so there is only one, and when `ADMIN_EMAIL` is set only that address can create it.
- A successful login creates an opaque random bearer token; only its SHA-256 hash is stored. Sessions expire after `SESSION_TTL_DAYS`. The time a session or token was last used is written at most once a minute, so it is accurate to the minute only.
- Personal API tokens (`ss_` prefix) use the same table with kind `token`, never expire, and can be revoked. They exist for the MCP server and scripts. A token acts with the whole account, including an administrator's or manager's role, except that only a signed-in session can create or revoke one, so a token given to a script cannot mint replacements for itself. An account holds at most 20, counted under a per-account lock.
- The per-network rate limits on the code endpoints are counted in each server process's memory, so several instances (serverless) each allow the full rate and a restart resets them. The per-email limits and the attempt count are stored in the database and hold across instances.
- Invited users get a real user row immediately so expenses can reference them; their first login activates the account and keeps all history.

## Consequences

### Positive

- No passwords to store, reset or leak.
- Mobile and web share one bearer-token scheme; no cookies means no CSRF surface.

### Negative

- Login depends on working email delivery. In development, codes are printed to the log.
- An API token is as powerful as its account: an administrator's token can change sign-up settings and disable accounts. People should give scripts a token from an account without a role where they can.

## Where It Is Enforced

| Claim | Code | Tests |
| ----- | ---- | ----- |
| Codes: 6 digits, 10 minutes, 5 attempts, HMAC with `SECRET_KEY` | `auth.DefaultPolicy`, `auth.Service.RequestCode`, `auth.Service.Verify` | `TestLoginFlow`, `TestAttemptsAreLimited` (`packages/core/domains/auth`) |
| Attempts counted in one statement; a code opens one session | `ChallengeRepository.Attempt`, `ChallengeRepository.Consume` | `TestVerifyAttemptsUnderConcurrency`, `TestCodeRedeemedOnce` (`packages/bootstrap`) |
| Same response for every address; per-network and per-email limits | `auth.Service.RequestCode` | `TestRequestCodeLooksTheSameForStrangers`, `TestCodeRequestsAreRateLimited`, `TestCodeRequestsAreLimitedPerEmail`, `TestStrangerCannotLockOutOwner`, `TestCodeRequestsUnderConcurrency` |
| Rules applied again on verify | `auth.Service.decide` | `TestVerifyAppliesTheRulesAgain`, `TestDisabledAccountCannotUseEarlierCode` |
| Modes; `domain` read as `invite` | `auth.Modes`, `auth.Normalize`, `auth.Policy.Signup`, `bootstrap.LoadConfig` | `TestNormalize`, `TestPolicySignupMapsDomainMode`, `TestDomainMode` |
| One first administrator; `ADMIN_EMAIL` | `auth.Service.decide`, `auth.Service.Verify` | `TestFirstAdminIsUnique`, `TestBootstrapRestrictedToAdminEmail` |
| Last use written at most once a minute | `auth.Service.Authenticate` | None; `TestTouchFailureIsLogged` covers a failed write |
| Tokens cannot manage tokens; at most 20 | `auth.Service.CreateToken` | `TestTokenCannotMintTokens`, `TestTokenLimit`, `TestTokensNeedSession` (`apps/api/routes`) |
| Per-network limit in process memory | `middleware.Limiter`, `apps/api/routes/router.go` | `TestLimit`, `TestSignInLimit` |
