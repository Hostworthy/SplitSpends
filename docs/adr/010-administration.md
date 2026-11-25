# 010: Administration in the App

## Status

Accepted

## Context

A server operator needs to decide who may join without editing environment variables and restarting: open the server for a weekend, keep strangers out, or approve people one by one. Some trusted friends should help with approvals without being able to change everything. Accounts sometimes have to be shut off, while the people still sharing expenses with them keep correct balances.

## Decision

- Three roles on the user: `user`, `manager` and `admin`. The first account is the administrator; it is decided and created under a lock so there is only one, and when `ADMIN_EMAIL` is set only that address can create it. Managers approve and decline people waiting to join; administrators also change sign-up settings and roles and disable accounts. The last active administrator cannot be removed or disabled, and nobody can disable themselves. Role changes, disabling and enabling run one at a time under a database lock, and check the acting administrator again under it, so changes made at the same moment cannot together remove every administrator.
- Sign-up modes are `open`, `invite`, `approval` and `closed`, stored in the database and changed in the app. `SIGNUP_MODE` and `SIGNUP_ALLOWED_DOMAINS` only give the starting values. An allowed list of addresses and `@domains` lets people in without waiting in invite and approval modes; closed means nobody new.
- In invite mode a group invite link is also a pass to create an account on the server, and any member of a group can make one, without a role. This is deliberate: invite mode is for servers whose members bring their own friends and family, and a link is how a member brings someone they cannot add by email. An administrator who wants to admit each person themselves uses approval mode, where links do not create accounts; closed mode admits nobody new, links included. Links are bounded so that one member cannot hand out an open door: a link lasts 14 days, a group has at most 20 usable links at a time, every member sees and can revoke them all, and the links of a member who leaves the group or is disabled stop working.
- In approval mode a person still proves their email with a code. Verifying it files a request instead of opening a session, so nobody without approval ever holds a session. Approval is checked again when the code is verified, and the person is emailed once approved. An approval also counts in invite mode, because an administrator or manager let that person in personally, so switching from approval to invite does not strand the people approved later. While sign-up is closed nobody new can sign in, so approving is refused instead of emailing someone who cannot.
- Accounts are disabled, never deleted: sessions, API tokens, push devices, outstanding login codes and the person's request to join are removed, the email is replaced by its SHA-256 hash, and the name stays on shared expenses. The hash recognises the same address when an administrator enables the account again or someone adds that email.
- An enabled account is re-enabled (`user.User.Reenabled`) until the person signs in: its status is `invited`, but unlike people someone added by email it has no address, only the hash. Its address signs in whatever the sign-up mode, as the account did before it was disabled, and the address is stored again on that sign-in. Nobody can be emailed until then, so the person is not told they were enabled. Administrators find disabled and re-enabled accounts by searching for the whole address.
- The same rules sit behind the API, so the app, the web and MCP tools all get them; the app only hides what a role cannot use.

## Consequences

- Balances never change when an account is disabled.
- A hash of an email is pseudonymous, not anonymous: anyone who already knows the address can match it. It keeps the association the operator asked for without storing the address itself.
- Requests and roles are new administration surfaces in the API and MCP, protected by role checks in the domain.
