# HTTP API Contract

All endpoints live under `/api/v1`. JSON field names are **camelCase**. The same contract is used by the Flutter app, the MCP server, and any third-party client.

## Conventions

### Authentication

Send `Authorization: Bearer <token>`. Tokens come from the OTP login flow (sessions) or from **Settings → API tokens** (long-lived personal tokens, used by the MCP server). Unauthenticated endpoints are marked **public**.

### Money

Amounts are **integers in minor units** of their currency (`1250` in `USD` is `$12.50`, `1250` in `JPY` is `¥1250`). The exponent per currency is published by `GET /config` (`currencies[].exponent`). Clients must never send floats for money.

Balance sign convention, always from the point of view of the caller unless noted: **positive means you are owed**, negative means you owe.

### Currencies

Every group has one currency (`defaultCurrency`), and expenses outside groups use the user's own `defaultCurrency`. The app never asks for a currency per expense: `currency` may be left out of `POST /expenses` and `POST /payments`, and the server fills it in that way. API clients may still send one, and older data can hold several currencies, so balances stay per currency and are never converted. Changing a group's currency affects new expenses only.

When AI reads amounts in another currency (a receipt from abroad), `POST /ai/expense-draft` converts them to the target currency with that day's exchange rate (`RATES_URL`), exactly and line by line: `minor × 10^(exp(to) − exp(from)) × rate(to) / rate(from)`, rounded half away from zero. A warning in the draft says which rate was used and what the receipt total was. If no rate is available the request fails with `RATES_UNAVAILABLE` rather than guessing. `spec/vectors.json` (`conversions`) pins the rule.

### Envelopes

| Kind | Status | Body |
| ---- | ------ | ---- |
| Details | 200/201 | `{ "data": T }` |
| List | 200 | `{ "items": T[], "total": number }` |
| Delete | 204 | empty |
| Error | 4xx/5xx | `{ "error": string, "code": string, "field"?: string, "requestId"?: string }` |

Error codes: `VALIDATION_ERROR` (422), `NOT_FOUND` (404), `UNAUTHORIZED` (401), `FORBIDDEN` (403), `CONFLICT` (409), `RATE_LIMITED` (429), `AI_DISABLED` (503), `RATES_UNAVAILABLE` (503), `APPROVAL_PENDING` (403, the account waits for an administrator), `APPROVAL_DECLINED` (403), `INTERNAL` (500). Several codes share a status, so clients read `code`, never the status alone; a 502, 503 or 504 without the error envelope comes from a proxy in front of the API (a timeout or an outage), not from the API.

An object the caller may not see answers 404 `NOT_FOUND`, exactly as an id that does not exist, so ids of other people's groups, expenses, comments, categories and people reveal nothing. 403 `FORBIDDEN` is only for objects the caller can see but may not change.

Lists accept `limit` (default 50, max 200) and `offset` (default 0).

Dates are `YYYY-MM-DD`; timestamps are RFC 3339 UTC.

## Shared Shapes

```jsonc
// User (email is null for people added by name only)
{ "id": "uuid", "email": "a@b.c" | null, "name": "Ada", "defaultCurrency": "USD",
  "avatarColor": "#7C4DFF", "avatarUrl": "https://gravatar.com/avatar/<sha256>?s=160&d=404" | null,
  "status": "active" | "invited" | "placeholder" }
// avatarUrl is set only when the person turned on Gravatar (and the host allows it).
// It answers 404 when they have no Gravatar, so clients keep the initials as a fallback.
// An account an administrator disabled shows as "active" to everyone except
// administrators (/admin/accounts), whose Account shape has "status": "disabled".

// Me (User plus private settings). In notifications the top-level switches
// are email (as before push existed); push has the same keys.
{ ...User, "timezone": "Europe/Berlin", "gravatar": false, "role": "user" | "manager" | "admin",
  "notifications": { "expenseAdded": true, "expenseUpdated": true, "paymentReceived": true, "addedToGroup": true,
                     "reminders": true,
                     "push": { "expenseAdded": true, "expenseUpdated": true, "paymentReceived": true, "addedToGroup": true, "reminders": true } } }

// PushSubscription (one of your devices; the endpoint is only shown to you)
{ "id": "uuid", "platform": "web" | "unifiedpush", "label": "Firefox on Linux",
  "endpoint": "https://push.example.com/...", "createdAt": "RFC3339", "lastSuccessAt": "RFC3339" | null, "failures": 0 }

// Balance (one per currency, only non-zero entries are returned)
{ "currency": "EUR", "amount": 1250 }

// Debt (an edge of the settle-up graph)
{ "from": "uuid", "to": "uuid", "currency": "EUR", "amount": 1250 }

// Group
{ "id": "uuid", "name": "Lisbon trip", "kind": "home" | "trip" | "couple" | "other",
  "defaultCurrency": "EUR", "simplifyDebts": true, "emoji": "✈️", "color": "#00BFA5",
  "archived": false, "createdAt": "...", "updatedAt": "...",
  "members": [User], "balances": [Balance] /* caller's balance in this group */,
  "defaultSplit": DefaultSplit | null }

// DefaultSplit (see "Default Split" below); values are decimal strings
{ "method": "equal" | "percent" | "shares", "values": { "<userId>": "60" } }

// Category (GET /categories; GET /config lists built-ins without custom and mine)
{ "id": "food.dining" | "custom.<uuid>", "name": "Dining out", "group": "Food and drink",
  "icon": "restaurant", "custom": false, "mine": false }

// Expense (payments are expenses with kind = "payment")
{ "id": "uuid", "groupId": "uuid" | null, "kind": "expense" | "payment",
  "description": "Dinner", "category": "food.dining", "currency": "EUR",
  "total": 9000, "date": "2026-11-20", "notes": "",
  "splitMethod": "equal" | "exact" | "percent" | "shares" | "adjustment" | "itemized",
  "shares": [{ "userId": "uuid", "paid": 9000, "owed": 3000, "value": null | number }],
  "items": [{ "name": "Pizza", "amount": 2400, "userIds": ["uuid"] }],   // itemized only
  "tax": 0, "tip": 0,                                                    // itemized only
  "receiptId": "uuid" | null, "recurrence": "none" | "weekly" | "biweekly" | "monthly" | "yearly",
  "createdBy": "uuid", "updatedBy": "uuid", "createdAt": "...", "updatedAt": "...",
  "deleted": false, "commentCount": 2,
  "myBalance": -3000 /* caller's paid - owed in this expense */ }

// Activity
{ "id": "uuid", "kind": "expense.created" | "expense.updated" | "expense.deleted" | "expense.restored"
  | "expense.stopped" | "payment.created" | "comment.created" | "group.created" | "group.updated" | "group.deleted" | "group.member-added"
  | "group.member-removed" | "group.member-joined" | "friend.added" | "reminder.sent",
  "actor": User, "groupId": "uuid" | null, "expenseId": "uuid" | null,
  "summary": "Ada added “Dinner” in Lisbon trip", "data": { ... }, "createdAt": "..." }
```

### Split Input

`POST /expenses` and `PUT /expenses/{id}` take `payers` and `splits`. The server computes every `owed` amount; rounding remainders are spread one minor unit at a time, deterministically.

| `splitMethod` | `splits[].value` | Rule |
| ------------- | ---------------- | ---- |
| `equal` | omitted | Total split evenly between listed users |
| `exact` | minor units | Values must sum to `total` |
| `percent` | percentage, up to 4 decimals | Values must sum to `100` |
| `shares` | positive number | Proportional to shares |
| `adjustment` | minor units (may be negative) | Each user owes the adjustment, the rest is split evenly |
| `itemized` | omitted | `items[].userIds` share each item evenly; `tax` and `tip` are spread proportionally to each user's item subtotal; `total` must equal items + tax + tip |

Split values are plain decimals such as `60`, `-3.5`, `1.` or `.25`: at most 20 characters and four decimal places, without exponents, fractions, base prefixes or digit separators. Surrounding whitespace is trimmed, and the trimmed value is what the expense stores.

`payers` is a list of `{ "userId", "amount" }` whose amounts sum to `total`. Every user in `payers`, `splits` or `items` must be a group member (group expense) or the caller's friend (non-group expense), and the caller must be involved. Outside groups, everyone who ends up owing money must also be connected (friends, or members of a common group) with everyone who ends up getting money back, since the expense leaves those pairs owing each other; otherwise 422 on `splits`. A recurring expense outside groups stops repeating once that no longer holds.

An expense lists at most 200 payers and 200 people in `splits`; an itemized expense has at most 200 items with at most 200 people each (422 on `payers`, `splits` or `items` otherwise).

`date` runs from `2000-01-01` up to one year after today (422 on `date` otherwise); an update that leaves it out keeps the expense's date.

A recurring expense repeats on its `recurrence`: a background job books each occurrence as a new expense, and the schedule moves to the newest copy, so editing the latest expense edits the series. Monthly and yearly series keep the day of the month they started on; in a shorter month they fall on its last day and return to their day afterwards (a series from 31 January books 28 February, then 31 March). Changing the date of the latest expense moves the series to the new day. When a series has fallen behind, only the 12 most recent missed occurrences are booked and older ones are skipped. A series ends instead of repeating once someone in it has left the group (or, outside groups, is no longer connected to the person who created it); that is recorded as `expense.stopped`.

An update may send `updatedAt`: the `updatedAt` of the expense as the client last saw it, unchanged. When the expense has been changed (or deleted) since, the update is refused with 409 `CONFLICT` and nothing is saved, so one person's edit never silently replaces another's; reload the expense and edit again. Without `updatedAt` the update always replaces the current version. Updating a deleted expense is a 409 either way.

`category` is a built-in id other than `payment`, or a custom category id (`custom.<uuid>`). A custom id must be one of the caller's own live categories; an update may also keep the category the expense already has, even when it belongs to another member or was deleted.

### Default Split

A group can store the split that new expenses in it start with. Clients apply it when someone adds an expense (the user can still change it); it never changes saved expenses. Only methods that do not depend on the total can be defaults.

| `method` | `values` | Rule |
| -------- | -------- | ---- |
| `equal` | `{}` | Everyone in the group, equally |
| `percent` | every current member, `"0"` leaves someone out | Plain decimals (at most four decimal places), never negative, adding up to exactly `100` |
| `shares` | every current member, `"0"` leaves someone out | Plain decimals (at most four decimal places), never negative, adding up to more than zero |

A `percent` or `shares` default applies only while its members are exactly the group's members. When someone joins or leaves, `Group.defaultSplit` is `null` (the default is treated as absent) until the group's members match it again or someone saves a new one, so a stale default never silently produces a wrong split. Violations return 422 with `field: "defaultSplit"`. The same rules run on the device (`packages/ledger`) and are tested with `spec/vectors.json`.

## Endpoints

### System

| Method | Path | Notes |
| ------ | ---- | ----- |
| GET | `/health` | **public**, outside `/api/v1`: `{ "ok": true }` |
| GET | `/config` | **public**: `{ data: { appName, aiEnabled, signupMode: "open" \| "invite" \| "domain", currencies: [{ code, name, symbol, exponent }], categories: [{ id, name, group, icon }], pushEnabled, vapidPublicKey, gravatarEnabled } }`. `vapidPublicKey` is the base64url VAPID key to subscribe with (empty when `pushEnabled` is false) |

### Auth

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/auth/otp/request` | `{ email, inviteToken? }` | **public**, 202 `{ data: { sent: true, expiresInSeconds: 600 } }` (identical whether or not the email exists, and whether or not the email could be sent). 429 `RATE_LIMITED` after 5 requests for an email in an hour from one network, or 10 from all networks, for every address alike. `inviteToken` is the token of a valid group invite link; it lets a new email sign up when `signupMode` would otherwise refuse it. An invalid token gets the same response and sends nothing |
| POST | `/auth/otp/verify` | `{ email, code, name? }` | **public**, 200 `{ data: { token, expiresAt, user: Me, isNew } }` |
| POST | `/auth/logout` | | 204, revokes the current session |

### Me

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/me` | | `{ data: Me }` |
| PATCH | `/me` | `{ name?, defaultCurrency?, timezone?, avatarColor?, gravatar?, notifications? }` | `{ data: Me }`. In `notifications`, an omitted `reminders` keeps its current value. `gravatar: true` needs an email and `gravatarEnabled` on the server |
| GET | `/me/balances` | | `{ data: { balances: [Balance], owed: [Balance], owing: [Balance] } }` totals across everything |
| GET | `/me/tokens` | | List of `{ id, name, createdAt, lastUsedAt }` |
| POST | `/me/tokens` | `{ name }` | 201 `{ data: { id, name, token, createdAt } }` (token shown once). Needs a signed-in session: an API token gets 403 `FORBIDDEN`. An account holds at most 20 tokens; the 21st is 422 `VALIDATION_ERROR` on `name` |
| DELETE | `/me/tokens/{tokenId}` | | 204. Needs a signed-in session: an API token gets 403 `FORBIDDEN` |
| GET | `/me/push-subscriptions` | | List of PushSubscription, newest first |
| POST | `/me/push-subscriptions` | `{ endpoint, keys: { p256dh, auth }, platform?: "web" \| "unifiedpush", label? }` | 201 `{ data: PushSubscription }`, or 200 when the endpoint was already registered (refreshed) |
| DELETE | `/me/push-subscriptions/{subscriptionId}` | | 204 |
| POST | `/me/push-subscriptions/test` | | `{ data: { sent, failed, removed } }` after sending a test notification to every device of the caller |

Notification preferences: `PATCH /me` with `notifications` merges, so omitted keys keep their value. `{ "notifications": { "push": { "paymentReceived": false } } }` turns off push for payments and leaves everything else as it was. Unknown kinds added later default to on.

Push subscriptions follow the Web Push standards: the body of `POST /me/push-subscriptions` is what `PushSubscription.toJSON()` returns in a browser (plus `platform` and `label`), and UnifiedPush on Android provides the same endpoint and keys. Rules: the endpoint must be an `https` URL (any port), and a URL whose host is an IP address the server would refuse to contact (see `PUSH_ALLOW_PRIVATE` in ENVIRONMENT.md) is rejected with 422; `p256dh` is an uncompressed P-256 public key and `auth` 16 bytes, both base64url. Registering is idempotent on the endpoint: the existing row is refreshed (its failure count is reset only when the keys changed), and it moves to the caller if another account registered it (the device now belongs to whoever is signed in). A device belongs to the session or API token that registered it (or last refreshed it): signing out (`POST /auth/logout`), revoking the token or the session expiring removes the device, so it stops getting the account's notifications. At most 25 devices per user, counting a device moved from another account (409 beyond that). `POST .../test` returns 409 when push is off or no device is registered, and waits at most 15 seconds. Each account may register devices 20 times a minute and send 5 test notifications a minute (429 `RATE_LIMITED`). When push is off on the server, `POST` returns 409.

Delivery: the server encrypts each message (RFC 8291, `aes128gcm`) and signs it with VAPID (RFC 8292), with a TTL of one day. The decrypted payload is small JSON, `{ "title": "Ada added “Rent”", "body": "Ada added “Rent” (€1,000.00) in Flat. Your share: €500.00.", "url": "<BASE_URL>" }`, with the same text as the email. A device the push service answers 404 or 410 for is removed; other failures, including no answer within 5 seconds, are counted (`failures`) and a device is removed after 25 failures in a row. Push is sent in the background, or before the response when `SERVERLESS=true`.

### Friends

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/friends` | | List of `{ user: User, balances: [Balance] }` (positive: friend owes you) |
| POST | `/friends` | `{ email?, name? }` | 201 `{ data: { user, balances } }`; with an email, creates an invited user when new and emails an invite; with only a name, creates a `placeholder` person |
| PATCH | `/friends/{userId}` | `{ email }` | `{ data: { user, balances } }`; gives a placeholder you created an email and invites them. 404 for anyone you did not add by name; 409 when the email already has an account (what was recorded for the placeholder cannot be moved to an existing account) or the person already has an email |
| GET | `/friends/{userId}` | | `{ data: { user, balances, groups: [{ groupId, groupName, balances }], nonGroup: [Balance] } }` |
| DELETE | `/friends/{userId}` | | 204; 409 when balances are not settled, or when you are connected only through a group you share (leave the group instead) |

Adding people sends email, so it is limited. `POST /friends`, `PATCH /friends/{userId}`, `POST /groups`, `POST /groups/{groupId}/members` and `POST /reminders` share a budget of 20 requests a minute per account (429 `RATE_LIMITED`). Each account may invite 20 new email addresses a day, counting every address that gets an invitation from these routes; a request that would go over answers 429 `RATE_LIMITED` and adds nobody. The invitation email is sent once the request has succeeded. Someone added to groups by the same person is emailed about it at most 3 times a day; later additions still reach their devices.

Each account may have up to 1000 people it added by name only (placeholders, from `/friends`, `/groups` and `/import`); adding another answers 409 `CONFLICT`. Giving a placeholder an email frees its place.

### Groups

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/groups` | `?archived=true` | List of `Group` |
| POST | `/groups` | `{ id?, name, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, defaultSplit?, members?: [{ email, name? }] }` | 201 `{ data: Group }`; `defaultSplit` is checked against the members after they are added |
| GET | `/groups/{groupId}` | | `{ data: Group }` |
| PATCH | `/groups/{groupId}` | `{ name?, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, archived?, defaultSplit? }` | `{ data: Group }`; `defaultSplit: null` clears it, leaving it out keeps it |
| DELETE | `/groups/{groupId}` | | 204; 403 when the caller may not delete it; 409 when not settled |
| POST | `/groups/{groupId}/members` | `{ email?, name? }` | 201 `{ data: Group }` (name only adds a placeholder) |
| DELETE | `/groups/{groupId}/members/{userId}` | | 204; 409 when that member is not settled. A member may remove themselves (leave). 409 when nobody who can sign in would be left (only people added by name remain): delete the group instead |
| GET | `/groups/{groupId}/balances` | | `{ data: { members: [{ userId, balances: [Balance] }], debts: [Debt] } }` (debts are simplified when `simplifyDebts` is on) |
| GET | `/groups/{groupId}/export` | | `text/csv` of all expenses and payments |
| POST | `/groups/{groupId}/invites` | | 201 `{ data: Invite }` with `token` and `url` (shown once). 409 for archived groups |
| GET | `/groups/{groupId}/invites` | | List of usable `Invite` (unexpired, created by a current member), without `token` and `url` |
| DELETE | `/groups/{groupId}/invites/{inviteId}` | | 204; the link stops working |

Deleting a group removes it for everyone, with all its expenses, payments, comments and activity. Only the person who created the group may delete it; once the creator is no longer a member, any member may. The group must be settled. The deletion is recorded as `group.deleted` (with `groupId: null` and the name in `data.groupName`) in the feed of everyone who was still a member, so it outlives the group.

A group has at most 100 members, its creator included. `POST /groups` with more `members` and `POST /groups/{groupId}/members` on a full group answer 422 `VALIDATION_ERROR` on `members`; joining a full group through an invite link answers 409 `CONFLICT`.

### Invite Links

Any group member can create a link that lets anyone who has it join the group. Links last 14 days and can be used many times until they expire or are revoked. A link stops working once the person who created it leaves or is removed from the group, so removing someone also ends the links they shared. The token is 32 random bytes in base64url; the server stores only its SHA-256 hash. `url` is `BASE_URL/join/<token>`, which opens the web app (QR codes encode it).

```jsonc
// Invite
{ "id": "uuid", "token": "base64url" /* create only */, "url": "https://split.example.com/join/<token>" /* create only */,
  "createdBy": "uuid", "expiresAt": "...", "createdAt": "..." }

// InvitePreview
{ "groupName": "Flat", "emoji": "🏠", "color": "#00897B", "memberCount": 3, "invitedBy": "Ada" }
```

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/invites/{token}` | | **public**, rate limited per IP: `{ data: InvitePreview }`. 404 for unknown, revoked and expired tokens, links whose creator has left the group or was disabled, and links to archived groups, alike |
| POST | `/invites/{token}/accept` | | `{ data: Group }`; adds the caller to the group and records `group.member-joined`. Accepting again, or as a member, returns the group unchanged |

People without an account follow the link, request a login code with `inviteToken`, sign in, then accept.

### Expenses and Payments

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/expenses` | `?groupId=&friendId=&q=&category=&from=&to=&kind=&includeDeleted=` | List of `Expense`, newest `date` first |
| POST | `/expenses` | `{ id?, groupId?, description, currency?, total, date, category?, notes?, splitMethod, payers, splits?, items?, tax?, tip?, receiptId?, recurrence? }` | 201 `{ data: Expense }` |
| GET | `/expenses/{expenseId}` | | `{ data: Expense }` |
| PUT | `/expenses/{expenseId}` | same as POST (full replace), plus `updatedAt?` | `{ data: Expense }`; 409 when `updatedAt` is set and the expense has changed since |
| DELETE | `/expenses/{expenseId}` | | 204 (soft delete); 409 when someone in it has left the group |
| POST | `/expenses/{expenseId}/restore` | | `{ data: Expense }`; 409 when someone in it has left the group |
| GET | `/expenses/{expenseId}/comments` | | List of `{ id, expenseId, user: User, body, createdAt }` |
| POST | `/expenses/{expenseId}/comments` | `{ body }` | 201 `{ data: Comment }` |
| DELETE | `/comments/{commentId}` | | 204 (author only; 403 for someone else's comment you can see, 404 when you cannot see the expense) |
| POST | `/payments` | `{ id?, groupId?, fromUserId, toUserId, amount, currency?, date?, notes? }` | 201 `{ data: Expense }` (kind `payment`) |

A payment is always one person paying another. `PUT /expenses/{id}` on a payment takes the usual expense body but must keep that shape: one entry in `payers` paying the whole `total` and one other person in `splits` (422 otherwise). Only the people, `total`, `currency`, `date` and `notes` are used; the description stays `Payment`, the category `payment`, and payments never recur. Only payments use the `payment` category.

Someone who has left a group still sees the group's expenses and payments they were part of, with their comments, but only current members may edit, delete, restore or comment on a group's expenses (404 otherwise). Outside groups, everyone in an expense may.

People leave a group only once they are settled, so a group expense or payment that includes someone who has left can no longer be deleted or restored (409 `CONFLICT`): either would give them a balance in a group they can no longer act in.

### Reminders

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/reminders` | `{ userId, groupId? }` | 201 `{ data: { id, userId, groupId, balances: [Balance], emailed, sentAt } }` |

Asks someone who owes you money to settle up: in one group when `groupId` is set, otherwise across everything you share. `balances` is what they owe you (positive amounts only, per currency). The reminder is emailed unless they turned the `reminders` notification off (`emailed: false`), and recorded as `reminder.sent` in both people's activity. Errors: 409 `CONFLICT` when they owe you nothing in that scope, or have no email address (people added by name only); 429 `RATE_LIMITED` when you already reminded them in the same scope in the last 24 hours, or sent them 3 reminders across all scopes in that time (enforced in the database); 422 when `groupId` is not one of your groups.

### Categories

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/categories` | | List of `Category`: the built-ins, then the caller's own custom categories and custom categories used by expenses the caller can see (`mine: false`), by name |
| POST | `/categories` | `{ id?, name, icon?, group? }` | 201 `{ data: Category }`; repeating a create with the same `id` returns the existing category; repeating it after you deleted the category brings it back with the details sent; an `id` owned by someone else returns 409 |
| PATCH | `/categories/{categoryId}` | `{ name?, icon?, group? }` | `{ data: Category }`; owner only (404 otherwise) |
| DELETE | `/categories/{categoryId}` | | 204; owner only |

Rules: `name` is 1 to 40 characters (whitespace collapsed). `icon` is one of `restaurant`, `local_grocery_store`, `local_bar`, `local_cafe`, `home`, `bolt`, `wifi`, `cleaning_services`, `chair`, `build`, `pets`, `local_gas_station`, `local_parking`, `local_taxi`, `directions_bus`, `directions_car`, `flight`, `hotel`, `attractions`, `movie`, `sports_esports`, `music_note`, `sports_soccer`, `subscriptions`, `checkroom`, `redeem`, `medical_services`, `school`, `child_care`, `shield`, `account_balance`, `shopping_bag`, `receipt_long`, `label`, `star`, `favorite`, `work`, `fitness_center`, `park`, `celebration`, `cake`, `savings`, `devices`, `menu_book`, `palette`, `volunteer_activism`, `local_laundry_service`, `content_cut` (default `label`). `group` is the display group, one of the built-in groups (`Food and drink`, `Home`, `Transportation`, `Travel`, `Entertainment`, `Life`, `Other`; default `Other`). Client-chosen ids look like `custom.<uuid>` (lowercase). Each person keeps up to 200 categories.

Deleting hides a category from listings and from new expenses but never rewrites history: expenses keep its id, clients show them as General, and `/stats` counts them under `general`. `GET /config` keeps returning only the built-ins.

### Activity and Stats

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/activity` | `?groupId=&expenseId=` | List of `Activity`, newest first |
| GET | `/stats` | `?groupId=&from=&to=&currency=` | `{ data: { currency, totalSpent, byCategory: [{ category, name, amount }], byMonth: [{ month: "2026-11", amount }], byMember: [{ userId, paid, owed }] } }`. `totalSpent` is your share (or the group total when `groupId` is set). Custom categories are grouped by id with their names; deleted ones count as `general` |

### Receipts and AI

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/receipts` | `multipart/form-data` field `file` (jpeg, png, webp, heic; max 10 MB) | 201 `{ data: { id, contentType, size, createdAt } }` |
| GET | `/receipts/{receiptId}` | | the image bytes |
| POST | `/receipts/{receiptId}/scan` | `{ currency? }` | `{ data: ReceiptScan }`; 503 `AI_DISABLED` when the host did not configure AI |
| POST | `/ai/expense-draft` | `{ text?, groupId?, receiptId? }` | `{ data: ExpenseDraft }` in the group's (or your) currency, converted when the receipt is in another one; 503 `AI_DISABLED` when the host did not configure AI, 503 `RATES_UNAVAILABLE` when a conversion is needed but no rate is available |

```jsonc
// ReceiptScan
{ "merchant": "Cafe Lua", "date": "2026-11-20" | null, "currency": "EUR",
  "items": [{ "name": "Pizza", "quantity": 1, "amount": 1200 }],
  "subtotal": 4000, "tax": 320, "tip": 400, "total": 4720, "category": "food.dining" }

// ExpenseDraft: a ready-to-review POST /expenses body (never saved automatically)
{ "groupId": "uuid" | null, "receiptId": "uuid" | null, "description": "Dinner", "currency": "EUR",
  "total": 5000, "date": "2026-11-24", "category": "food.dining",
  "splitMethod": "equal" | "itemized",
  "payers": [{ "userId": "uuid", "amount": 5000 }],
  "splits": [{ "userId": "uuid" }],                                    // equal drafts
  "items": [{ "name": "Pasta", "amount": 2000, "userIds": ["uuid"] }],  // itemized drafts
  "tax": 400, "tip": 0,
  "warnings": ["Could not match “Sam” to anyone you know."] }
```

**How drafts work.** Split Spends runs no model itself. The server sends the user's instructions (for example "Ben had the pasta, split the rest with everyone in Lisbon trip"), the receipt photo when `receiptId` is given, and a system prompt to the provider the host configured (see [ENVIRONMENT.md](ENVIRONMENT.md#ai-optional)). The system prompt lists today's date, the user's people and groups with their ids, group members and currencies, the categories, and the rules for filling the answer; the provider must answer in a fixed JSON schema. The server then checks the answer: ids the user cannot use are dropped (with a warning), people outside the chosen group are left out, item totals are reconciled with the receipt total, and the result is a valid expense body. A draft becomes an expense only when the user saves it. `POST /receipts/{receiptId}/scan` remains for clients that only want the raw receipt lines.

### Offline Clients and Import

`POST /groups`, `POST /expenses` and `POST /payments` accept an optional client-generated `id` (UUID). Repeating the request with the same `id` returns the existing object as it is now (edited, or with `deleted: true` when it was deleted since) instead of creating a duplicate, so an offline outbox can retry safely, also while the first request is still running. An `id` that belongs to something the caller cannot see returns 409, and so does an `id` used for a different kind of object: an expense `id` sent to `/payments` or a payment `id` to `/expenses`, or a retry naming another `groupId`. Every such 409 has the same message, whatever the `id` names.

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/import` | `ImportRequest` (max 16 MB) | `{ data: { people: { localId: serverUserId }, categories, groups, expenses } }` |

```jsonc
// ImportRequest: everything recorded in local-only mode. All ids are UUIDs chosen by the device.
{ "meId": "uuid",                       // the local person that stands for the device owner
  "people": [{ "id": "uuid", "name": "Sam", "email": "sam@x.com" /* optional */ }],
  "groups": [{ "id": "uuid", "name": "Camping", "kind": "trip", "defaultCurrency": "EUR",
               "simplifyDebts": true, "emoji": "⛺", "color": "#00897B", "archived": false,
               "memberIds": ["uuid"] }],
  "categories": [{ "id": "custom.<uuid>", "name": "Climbing", "icon": "fitness_center",
                   "group": "Entertainment", "deleted": false }],     // optional
  "expenses": [{ "id": "uuid", "kind": "expense" | "payment", ...ExpenseInput }] }
// groups[].defaultSplit is optional; its values are keyed by local person ids
```

Rules: runs in one transaction and sends no emails. People with an email resolve to the existing (or a newly invited) user, so their server id may differ from the local id: use the returned `people` map to rewrite local references. People without an email become placeholders that keep the local id. The caller becomes friends with everyone imported. Payments use `kind: "payment"` with one payer and one split (the recipient). Custom categories are created first with their device ids, so expenses can use them; deleted ones are created too and deleted once the expenses are in. A group's `defaultSplit` has its person ids remapped like `memberIds` and must match the group's members (send it only while it still applies). Two people in the bundle who resolve to the same server user (the same email, or your own email) are refused with 422 when both appear in one default split, among one expense's payers or splits, or in one item; as group members they simply join once. Validation errors name the failing item in `field`, for example `expenses[3].total`. Retrying the same import is safe.

Limits: one import may hold at most 1000 people, 200 custom categories, 500 groups and 20,000 expenses. Each account runs one import at a time (409 `CONFLICT` while another is running) and at most 10 an hour (429 `RATE_LIMITED`); the server runs at most two imports at once and answers 429 `RATE_LIMITED` to a third until one ends.

### Administration

Roles: `user`, `manager` (approves and declines people waiting to join) and `admin` (everything below). The first account on a server is an administrator; the last active administrator cannot step down or be disabled, and nobody can disable themselves.

Sign-up modes:

| Mode | Who can create an account |
| --- | --- |
| `open` | Anyone |
| `invite` | The allowed list, people a member added by email, holders of a group invite link, and people an administrator or manager approved |
| `approval` | Anyone can ask: after proving their email with a code, `POST /auth/otp/verify` answers 403 `APPROVAL_PENDING` until an administrator or manager approves them (403 `APPROVAL_DECLINED` once declined). The allowed list and people approved earlier sign in directly. Invited people also wait |
| `closed` | Nobody new, not even the allowed list |

Existing active accounts always sign in. The allowed list holds addresses and whole domains (`@family.com`). `SIGNUP_MODE` and `SIGNUP_ALLOWED_DOMAINS` are the starting values until an administrator saves settings in the app.

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| GET | `/admin/signup` | | admin: `{ data: { mode, allowed: [], updatedAt } }` |
| PUT | `/admin/signup` | `{ mode, allowed: [] }` | admin: the saved settings (entries lowercased, `example.com` becomes `@example.com`) |
| GET | `/admin/requests` | `?status=pending\|approved\|declined&limit=&offset=` (pending by default) | admin or manager: list of `{ id, email, name, status, requestedAt, decidedAt }`, newest first |
| POST | `/admin/requests/{requestId}/approve` | | admin or manager: the request; the person is emailed and can sign in. 409 while the sign-up mode is `closed` |
| POST | `/admin/requests/{requestId}/decline` | | admin or manager: the request |
| GET | `/admin/accounts` | `?status=active\|invited\|disabled&q=&limit=&offset=` | admin: list of `{ ...User, role, createdAt }` (everyone except people added by name only) |
| PATCH | `/admin/accounts/{userId}` | `{ role }` | admin: the account |
| POST | `/admin/accounts/{userId}/disable` | | admin: removes sessions, API tokens and push devices, deletes the email (keeping its SHA-256 hash so the same address is recognised) and sets `status: "disabled"`. The name stays on shared expenses |
| POST | `/admin/accounts/{userId}/enable` | | admin: the person signs in again with their old email, which is stored again |

Administrators and managers get an `approvals` notification (email and push) for a new request, unless another request was already waiting from less than an hour before; a burst of requests sends one notification. At most 500 requests wait at a time: beyond that, signing in as someone new answers 429 `RATE_LIMITED` until administrators decide some. A request nobody decides within 30 days is removed by the cleanup job, and the person can ask again.

### Maintenance

| Method | Path | Notes |
| ------ | ---- | ----- |
| POST | `/internal/jobs` | Outside `/api/v1`. Runs recurring expenses and cleanup once; overlapping runs are safe, each occurrence is booked once. Every job runs even when another fails, and a recurring expense that fails is skipped and tried again next run; any failure answers 500 after the rest has run. Requires `Authorization: Bearer <JOBS_SECRET>`; only mounted when `JOBS_SECRET` is set. For serverless schedulers |

## MCP

The MCP server speaks Streamable HTTP at `/mcp` on the same host (send the same `Authorization: Bearer <api token>` header) and stdio via the `splitspends-mcp` binary. See [MCP.md](MCP.md).
