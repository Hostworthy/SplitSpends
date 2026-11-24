# HTTP API Contract

All endpoints live under `/api/v1`. JSON field names are **camelCase**. The same contract is used by the Flutter app, the MCP server, and any third-party client.

## Conventions

### Authentication

Send `Authorization: Bearer <token>`. Tokens come from the OTP login flow (sessions) or from **Settings → API tokens** (long-lived personal tokens, used by the MCP server). Unauthenticated endpoints are marked **public**.

### Money

Amounts are **integers in minor units** of their currency (`1250` in `USD` is `$12.50`, `1250` in `JPY` is `¥1250`). The exponent per currency is published by `GET /config` (`currencies[].exponent`). Clients must never send floats for money.

Balance sign convention, always from the point of view of the caller unless noted: **positive means you are owed**, negative means you owe.

### Envelopes

| Kind | Status | Body |
| ---- | ------ | ---- |
| Details | 200/201 | `{ "data": T }` |
| List | 200 | `{ "items": T[], "total": number }` |
| Delete | 204 | empty |
| Error | 4xx/5xx | `{ "error": string, "code": string, "field"?: string, "requestId"?: string }` |

Error codes: `VALIDATION_ERROR` (422), `NOT_FOUND` (404), `UNAUTHORIZED` (401), `FORBIDDEN` (403), `CONFLICT` (409), `RATE_LIMITED` (429), `AI_DISABLED` (503), `INTERNAL` (500).

Lists accept `limit` (default 50, max 200) and `offset` (default 0).

Dates are `YYYY-MM-DD`; timestamps are RFC 3339 UTC.

## Shared Shapes

```jsonc
// User
{ "id": "uuid", "email": "a@b.c", "name": "Ada", "defaultCurrency": "USD",
  "avatarColor": "#7C4DFF", "status": "active" | "invited" }

// Me (User plus private settings)
{ ...User, "timezone": "Europe/Berlin",
  "notifications": { "expenseAdded": true, "expenseUpdated": true, "paymentReceived": true, "addedToGroup": true } }

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
  | "group.member-removed" | "friend.added",
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
| POST | `/auth/otp/request` | `{ email }` | **public**, 202 `{ data: { sent: true, expiresInSeconds: 600 } }` (identical whether or not the email exists) |
| POST | `/auth/otp/verify` | `{ email, code, name? }` | **public**, 200 `{ data: { token, expiresAt, user: Me, isNew } }` |
| POST | `/auth/logout` | | 204, revokes the current session |

### Me

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/me` | | `{ data: Me }` |
| PATCH | `/me` | `{ name?, defaultCurrency?, timezone?, avatarColor?, notifications? }` | `{ data: Me }` |
| GET | `/me/balances` | | `{ data: { balances: [Balance], owed: [Balance], owing: [Balance] } }` totals across everything |
| GET | `/me/tokens` | | List of `{ id, name, createdAt, lastUsedAt }` |
| POST | `/me/tokens` | `{ name }` | 201 `{ data: { id, name, token, createdAt } }` (token shown once) |
| DELETE | `/me/tokens/{tokenId}` | | 204 |

### Friends

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/friends` | | List of `{ user: User, balances: [Balance] }` (positive: friend owes you) |
| POST | `/friends` | `{ email, name? }` | 201 `{ data: { user, balances } }`; creates an invited user when the email is new and emails an invite |
| GET | `/friends/{userId}` | | `{ data: { user, balances, groups: [{ groupId, groupName, balances }], nonGroup: [Balance] } }` |
| DELETE | `/friends/{userId}` | | 204; 409 when balances are not settled |

### Groups

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/groups` | `?archived=true` | List of `Group` |
| POST | `/groups` | `{ name, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, members?: [{ email, name? }] }` | 201 `{ data: Group }` |
| GET | `/groups/{groupId}` | | `{ data: Group }` |
| PATCH | `/groups/{groupId}` | `{ name?, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, archived? }` | `{ data: Group }` |
| DELETE | `/groups/{groupId}` | | 204; 409 when not settled |
| POST | `/groups/{groupId}/members` | `{ email, name? }` | 201 `{ data: Group }` |
| DELETE | `/groups/{groupId}/members/{userId}` | | 204; 409 when that member is not settled. A member may remove themselves (leave) |
| GET | `/groups/{groupId}/balances` | | `{ data: { members: [{ userId, balances: [Balance] }], debts: [Debt] } }` (debts are simplified when `simplifyDebts` is on) |
| GET | `/groups/{groupId}/export` | | `text/csv` of all expenses and payments |

### Expenses and Payments

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/expenses` | `?groupId=&friendId=&q=&category=&from=&to=&kind=&includeDeleted=` | List of `Expense`, newest `date` first |
| POST | `/expenses` | `{ groupId?, description, currency, total, date, category?, notes?, splitMethod, payers, splits?, items?, tax?, tip?, receiptId?, recurrence? }` | 201 `{ data: Expense }` |
| GET | `/expenses/{expenseId}` | | `{ data: Expense }` |
| PUT | `/expenses/{expenseId}` | same as POST (full replace) | `{ data: Expense }` |
| DELETE | `/expenses/{expenseId}` | | 204 (soft delete) |
| POST | `/expenses/{expenseId}/restore` | | `{ data: Expense }` |
| GET | `/expenses/{expenseId}/comments` | | List of `{ id, expenseId, user: User, body, createdAt }` |
| POST | `/expenses/{expenseId}/comments` | `{ body }` | 201 `{ data: Comment }` |
| DELETE | `/comments/{commentId}` | | 204 (author only) |
| POST | `/payments` | `{ groupId?, fromUserId, toUserId, amount, currency, date?, notes? }` | 201 `{ data: Expense }` (kind `payment`) |

### Activity and Stats

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/activity` | `?groupId=` | List of `Activity`, newest first |
| GET | `/stats` | `?groupId=&from=&to=&currency=` | `{ data: { currency, totalSpent, byCategory: [{ category, amount }], byMonth: [{ month: "2026-11", amount }], byMember: [{ userId, paid, owed }] } }`. `totalSpent` is your share (or the group total when `groupId` is set) |

### Receipts and AI

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| POST | `/receipts` | `multipart/form-data` field `file` (jpeg, png, webp, heic; max 10 MB) | 201 `{ data: { id, contentType, size, createdAt } }` |
| GET | `/receipts/{receiptId}` | | the image bytes |
| POST | `/receipts/{receiptId}/scan` | `{ currency? }` | `{ data: ReceiptScan }`; 503 `AI_DISABLED` when the host did not configure AI |
| POST | `/ai/expense-draft` | `{ text, groupId? }` | `{ data: ExpenseDraft }` turns "dinner 60 paid by me split with Sam" into a draft |

```jsonc
// ReceiptScan
{ "merchant": "Cafe Lua", "date": "2026-11-20" | null, "currency": "EUR",
  "items": [{ "name": "Pizza", "quantity": 1, "amount": 1200 }],
  "subtotal": 4000, "tax": 320, "tip": 400, "total": 4720, "category": "food.dining" }

// ExpenseDraft: a ready-to-review POST /expenses body (never saved automatically)
{ "groupId": "uuid" | null, "description": "Dinner", "currency": "EUR", "total": 6000,
  "date": "2026-11-24", "category": "food.dining", "splitMethod": "equal",
  "payers": [{ "userId": "uuid", "amount": 6000 }], "splits": [{ "userId": "uuid" }],
  "warnings": ["Could not match “Sam” to a friend"] }
```

## MCP

The MCP server speaks Streamable HTTP at `/mcp` on the same host (send the same `Authorization: Bearer <api token>` header) and stdio via the `splitspends-mcp` binary. See [MCP.md](MCP.md).
