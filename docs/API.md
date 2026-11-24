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

Error codes: `VALIDATION_ERROR` (422), `NOT_FOUND` (404), `UNAUTHORIZED` (401), `FORBIDDEN` (403), `CONFLICT` (409), `RATE_LIMITED` (429), `AI_DISABLED` (503), `RATES_UNAVAILABLE` (503), `INTERNAL` (500).

Lists accept `limit` (default 50, max 200) and `offset` (default 0).

Dates are `YYYY-MM-DD`; timestamps are RFC 3339 UTC.

## Shared Shapes

```jsonc
// User (email is null for people added by name only)
{ "id": "uuid", "email": "a@b.c" | null, "name": "Ada", "defaultCurrency": "USD",
  "avatarColor": "#7C4DFF", "status": "active" | "invited" | "placeholder" }

// Me (User plus private settings)
{ ...User, "timezone": "Europe/Berlin",
  "notifications": { "expenseAdded": true, "expenseUpdated": true, "paymentReceived": true, "addedToGroup": true,
                     "reminders": true } }

// Balance (one per currency, only non-zero entries are returned)
{ "currency": "EUR", "amount": 1250 }

// Debt (an edge of the settle-up graph)
{ "from": "uuid", "to": "uuid", "currency": "EUR", "amount": 1250 }

// Group
{ "id": "uuid", "name": "Lisbon trip", "kind": "home" | "trip" | "couple" | "other",
  "defaultCurrency": "EUR", "simplifyDebts": true, "emoji": "✈️", "color": "#00BFA5",
  "archived": false, "createdAt": "...", "updatedAt": "...",
  "members": [User], "balances": [Balance] /* caller's balance in this group */ }

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
  | "payment.created" | "comment.created" | "group.created" | "group.updated" | "group.member-added"
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
| `percent` | percentage, up to 2 decimals | Values must sum to `100` |
| `shares` | positive number | Proportional to shares |
| `adjustment` | minor units (may be negative) | Each user owes the adjustment, the rest is split evenly |
| `itemized` | omitted | `items[].userIds` share each item evenly; `tax` and `tip` are spread proportionally to each user's item subtotal; `total` must equal items + tax + tip |

`payers` is a list of `{ "userId", "amount" }` whose amounts sum to `total`. Every user in `payers`, `splits` or `items` must be a group member (group expense) or the caller's friend (non-group expense), and the caller must be involved.

## Endpoints

### System

| Method | Path | Notes |
| ------ | ---- | ----- |
| GET | `/health` | **public**, outside `/api/v1`: `{ "ok": true }` |
| GET | `/config` | **public**: `{ data: { appName, aiEnabled, signupMode: "open" \| "invite" \| "domain", currencies: [{ code, name, symbol, exponent }], categories: [{ id, name, group, icon }] } }` |

### Auth

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/auth/otp/request` | `{ email, inviteToken? }` | **public**, 202 `{ data: { sent: true, expiresInSeconds: 600 } }` (identical whether or not the email exists). `inviteToken` is the token of a valid group invite link; it lets a new email sign up when `signupMode` would otherwise refuse it. An invalid token gets the same response and sends nothing |
| POST | `/auth/otp/verify` | `{ email, code, name? }` | **public**, 200 `{ data: { token, expiresAt, user: Me, isNew } }` |
| POST | `/auth/logout` | | 204, revokes the current session |

### Me

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/me` | | `{ data: Me }` |
| PATCH | `/me` | `{ name?, defaultCurrency?, timezone?, avatarColor?, notifications? }` | `{ data: Me }`. In `notifications`, an omitted `reminders` keeps its current value |
| GET | `/me/balances` | | `{ data: { balances: [Balance], owed: [Balance], owing: [Balance] } }` totals across everything |
| GET | `/me/tokens` | | List of `{ id, name, createdAt, lastUsedAt }` |
| POST | `/me/tokens` | `{ name }` | 201 `{ data: { id, name, token, createdAt } }` (token shown once) |
| DELETE | `/me/tokens/{tokenId}` | | 204 |

### Friends

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/friends` | | List of `{ user: User, balances: [Balance] }` (positive: friend owes you) |
| POST | `/friends` | `{ email?, name? }` | 201 `{ data: { user, balances } }`; with an email, creates an invited user when new and emails an invite; with only a name, creates a `placeholder` person |
| PATCH | `/friends/{userId}` | `{ email }` | `{ data: { user, balances } }`; gives a placeholder you created an email and invites them. 409 when the email already has an account |
| GET | `/friends/{userId}` | | `{ data: { user, balances, groups: [{ groupId, groupName, balances }], nonGroup: [Balance] } }` |
| DELETE | `/friends/{userId}` | | 204; 409 when balances are not settled |

### Groups

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/groups` | `?archived=true` | List of `Group` |
| POST | `/groups` | `{ id?, name, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, members?: [{ email, name? }] }` | 201 `{ data: Group }` |
| GET | `/groups/{groupId}` | | `{ data: Group }` |
| PATCH | `/groups/{groupId}` | `{ name?, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, archived? }` | `{ data: Group }` |
| DELETE | `/groups/{groupId}` | | 204; 409 when not settled |
| POST | `/groups/{groupId}/members` | `{ email?, name? }` | 201 `{ data: Group }` (name only adds a placeholder) |
| DELETE | `/groups/{groupId}/members/{userId}` | | 204; 409 when that member is not settled. A member may remove themselves (leave) |
| GET | `/groups/{groupId}/balances` | | `{ data: { members: [{ userId, balances: [Balance] }], debts: [Debt] } }` (debts are simplified when `simplifyDebts` is on) |
| GET | `/groups/{groupId}/export` | | `text/csv` of all expenses and payments |
| POST | `/groups/{groupId}/invites` | | 201 `{ data: Invite }` with `token` and `url` (shown once). 409 for archived groups |
| GET | `/groups/{groupId}/invites` | | List of active (unexpired) `Invite`, without `token` and `url` |
| DELETE | `/groups/{groupId}/invites/{inviteId}` | | 204; the link stops working |

### Invite Links

Any group member can create a link that lets anyone who has it join the group. Links last 14 days and can be used many times until they expire or are revoked. The token is 32 random bytes in base64url; the server stores only its SHA-256 hash. `url` is `BASE_URL/join/<token>`, which opens the web app (QR codes encode it).

```jsonc
// Invite
{ "id": "uuid", "token": "base64url" /* create only */, "url": "https://split.example.com/join/<token>" /* create only */,
  "createdBy": "uuid", "expiresAt": "...", "createdAt": "..." }

// InvitePreview
{ "groupName": "Flat", "emoji": "🏠", "color": "#00897B", "memberCount": 3, "invitedBy": "Ada" }
```

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/invites/{token}` | | **public**, rate limited per IP: `{ data: InvitePreview }`. 404 for unknown, revoked and expired tokens alike |
| POST | `/invites/{token}/accept` | | `{ data: Group }`; adds the caller to the group and records `group.member-joined`. Accepting again, or as a member, returns the group unchanged |

People without an account follow the link, request a login code with `inviteToken`, sign in, then accept.

### Expenses and Payments

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/expenses` | `?groupId=&friendId=&q=&category=&from=&to=&kind=&includeDeleted=` | List of `Expense`, newest `date` first |
| POST | `/expenses` | `{ id?, groupId?, description, currency?, total, date, category?, notes?, splitMethod, payers, splits?, items?, tax?, tip?, receiptId?, recurrence? }` | 201 `{ data: Expense }` |
| GET | `/expenses/{expenseId}` | | `{ data: Expense }` |
| PUT | `/expenses/{expenseId}` | same as POST (full replace) | `{ data: Expense }` |
| DELETE | `/expenses/{expenseId}` | | 204 (soft delete) |
| POST | `/expenses/{expenseId}/restore` | | `{ data: Expense }` |
| GET | `/expenses/{expenseId}/comments` | | List of `{ id, expenseId, user: User, body, createdAt }` |
| POST | `/expenses/{expenseId}/comments` | `{ body }` | 201 `{ data: Comment }` |
| DELETE | `/comments/{commentId}` | | 204 (author only) |
| POST | `/payments` | `{ id?, groupId?, fromUserId, toUserId, amount, currency?, date?, notes? }` | 201 `{ data: Expense }` (kind `payment`) |

### Reminders

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/reminders` | `{ userId, groupId? }` | 201 `{ data: { id, userId, groupId, balances: [Balance], emailed, sentAt } }` |

Asks someone who owes you money to settle up: in one group when `groupId` is set, otherwise across everything you share. `balances` is what they owe you (positive amounts only, per currency). The reminder is emailed unless they turned the `reminders` notification off (`emailed: false`), and recorded as `reminder.sent` in both people's activity. Errors: 409 `CONFLICT` when they owe you nothing in that scope, or have no email address (people added by name only); 429 `RATE_LIMITED` when you already reminded them in the same scope in the last 24 hours (enforced in the database); 422 when `groupId` is not one of your groups.

### Activity and Stats

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/activity` | `?groupId=&expenseId=` | List of `Activity`, newest first |
| GET | `/stats` | `?groupId=&from=&to=&currency=` | `{ data: { currency, totalSpent, byCategory: [{ category, amount }], byMonth: [{ month: "2026-11", amount }], byMember: [{ userId, paid, owed }] } }`. `totalSpent` is your share (or the group total when `groupId` is set) |

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

**How drafts work.** SplitSpends runs no model itself. The server sends the user's instructions (for example "Ben had the pasta, split the rest with everyone in Lisbon trip"), the receipt photo when `receiptId` is given, and a system prompt to the provider the host configured (see [ENVIRONMENT.md](ENVIRONMENT.md#ai-optional)). The system prompt lists today's date, the user's people and groups with their ids, group members and currencies, the categories, and the rules for filling the answer; the provider must answer in a fixed JSON schema. The server then checks the answer: ids the user cannot use are dropped (with a warning), people outside the chosen group are left out, item totals are reconciled with the receipt total, and the result is a valid expense body. A draft becomes an expense only when the user saves it. `POST /receipts/{receiptId}/scan` remains for clients that only want the raw receipt lines.

### Offline Clients and Import

`POST /groups`, `POST /expenses` and `POST /payments` accept an optional client-generated `id` (UUID). Repeating the request with the same `id` returns the existing object instead of creating a duplicate, so an offline outbox can retry safely. An `id` that belongs to something the caller cannot see returns 409.

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/import` | `ImportRequest` (max 16 MB) | `{ data: { people: { localId: serverUserId }, groups, expenses } }` |

```jsonc
// ImportRequest: everything recorded in local-only mode. All ids are UUIDs chosen by the device.
{ "meId": "uuid",                       // the local person that stands for the device owner
  "people": [{ "id": "uuid", "name": "Sam", "email": "sam@x.com" /* optional */ }],
  "groups": [{ "id": "uuid", "name": "Camping", "kind": "trip", "defaultCurrency": "EUR",
               "simplifyDebts": true, "emoji": "⛺", "color": "#00897B", "archived": false,
               "memberIds": ["uuid"] }],
  "expenses": [{ "id": "uuid", "kind": "expense" | "payment", ...ExpenseInput }] }
```

Rules: runs in one transaction and sends no emails. People with an email resolve to the existing (or a newly invited) user, so their server id may differ from the local id: use the returned `people` map to rewrite local references. People without an email become placeholders that keep the local id. The caller becomes friends with everyone imported. Payments use `kind: "payment"` with one payer and one split (the recipient). Validation errors name the failing item in `field`, for example `expenses[3].total`. Retrying the same import is safe.

### Maintenance

| Method | Path | Notes |
| ------ | ---- | ----- |
| POST | `/internal/jobs` | Outside `/api/v1`. Runs recurring expenses and cleanup once. Requires `Authorization: Bearer <JOBS_SECRET>`; only mounted when `JOBS_SECRET` is set. For serverless schedulers |

## MCP

The MCP server speaks Streamable HTTP at `/mcp` on the same host (send the same `Authorization: Bearer <api token>` header) and stdio via the `splitspends-mcp` binary. See [MCP.md](MCP.md).
