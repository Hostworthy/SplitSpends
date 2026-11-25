# 010: Administration in the App

## Status

Accepted

## Context

A server operator needs to decide who may join without editing environment variables and restarting: open the server for a weekend, keep strangers out, or approve people one by one. Some trusted friends should help with approvals without being able to change everything. Accounts sometimes have to be shut off, while the people still sharing expenses with them keep correct balances.

## Decision

- Three roles on the user: `user`, `manager` and `admin`. The first account is the administrator; it is decided and created under a lock so there is only one, and when `ADMIN_EMAIL` is set only that address can create it. Managers approve and decline people waiting to join; administrators also change sign-up settings and roles and disable accounts. The last active administrator cannot be removed or disabled, and nobody can disable themselves. Role changes, disabling and enabling run one at a time under a database lock, and check the acting administrator again under it, so changes made at the same moment cannot together remove every administrator.
- Sign-up modes are `open`, `invite`, `approval` and `closed`, stored in the database and changed in the app. `SIGNUP_MODE` and `SIGNUP_ALLOWED_DOMAINS` only give the starting values. An allowed list of addresses and `@domains` lets people in without waiting in invite and approval modes; closed means nobody new.
- In approval mode a person still proves their email with a code. Verifying it files a request instead of opening a session, so nobody without approval ever holds a session. Approval is checked again when the code is verified, and the person is emailed once approved.
- Accounts are disabled, never deleted: sessions, API tokens, push devices, outstanding login codes and the person's request to join are removed, the email is replaced by its SHA-256 hash, and the name stays on shared expenses. The hash recognises the same address when an administrator enables the account again or someone adds that email.
- The same rules sit behind the API, so the app, the web and MCP tools all get them; the app only hides what a role cannot use.

## Consequences

- Balances never change when an account is disabled.
- A hash of an email is pseudonymous, not anonymous: anyone who already knows the address can match it. It keeps the association the operator asked for without storing the address itself.
- Requests and roles are new administration surfaces in the API and MCP, protected by role checks in the domain.
