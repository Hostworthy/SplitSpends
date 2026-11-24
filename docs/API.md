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
// User (email is null for people added by name only)
{ "id": "uuid", "email": "a@b.c" | null, "name": "Ada", "defaultCurrency": "USD",
  "avatarColor": "#7C4DFF", "status": "active" | "invited" | "placeholder" }

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

`category` is a built-in id or a custom category id (`custom.<uuid>`). A custom id must be one of the caller's own live categories; an update may also keep the category the expense already has, even when it belongs to another member or was deleted.

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
| POST | `/friends` | `{ email?, name? }` | 201 `{ data: { user, balances } }`; with an email, creates an invited user when new and emails an invite; with only a name, creates a `placeholder` person |
| PATCH | `/friends/{userId}` | `{ email }` | `{ data: { user, balances } }`; gives a placeholder you created an email and invites them. 409 when the email already has an account |
| GET | `/friends/{userId}` | | `{ data: { user, balances, groups: [{ groupId, groupName, balances }], nonGroup: [Balance] } }` |
| DELETE | `/friends/{userId}` | | 204; 409 when balances are not settled |

### Groups

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/groups` | `?archived=true` | List of `Group` |
| POST | `/groups` | `{ id?, name, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, defaultSplit?, members?: [{ email, name? }] }` | 201 `{ data: Group }`; `defaultSplit` is checked against the members after they are added |
| GET | `/groups/{groupId}` | | `{ data: Group }` |
| PATCH | `/groups/{groupId}` | `{ name?, kind?, defaultCurrency?, simplifyDebts?, emoji?, color?, archived?, defaultSplit? }` | `{ data: Group }`; `defaultSplit: null` clears it, leaving it out keeps it |
| DELETE | `/groups/{groupId}` | | 204; 409 when not settled |
| POST | `/groups/{groupId}/members` | `{ email?, name? }` | 201 `{ data: Group }` (name only adds a placeholder) |
| DELETE | `/groups/{groupId}/members/{userId}` | | 204; 409 when that member is not settled. A member may remove themselves (leave) |
| GET | `/groups/{groupId}/balances` | | `{ data: { members: [{ userId, balances: [Balance] }], debts: [Debt] } }` (debts are simplified when `simplifyDebts` is on) |
| GET | `/groups/{groupId}/export` | | `text/csv` of all expenses and payments |

### Expenses and Payments

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/expenses` | `?groupId=&friendId=&q=&category=&from=&to=&kind=&includeDeleted=` | List of `Expense`, newest `date` first |
| POST | `/expenses` | `{ id?, groupId?, description, currency, total, date, category?, notes?, splitMethod, payers, splits?, items?, tax?, tip?, receiptId?, recurrence? }` | 201 `{ data: Expense }` |
| GET | `/expenses/{expenseId}` | | `{ data: Expense }` |
| PUT | `/expenses/{expenseId}` | same as POST (full replace) | `{ data: Expense }` |
| DELETE | `/expenses/{expenseId}` | | 204 (soft delete) |
| POST | `/expenses/{expenseId}/restore` | | `{ data: Expense }` |
| GET | `/expenses/{expenseId}/comments` | | List of `{ id, expenseId, user: User, body, createdAt }` |
| POST | `/expenses/{expenseId}/comments` | `{ body }` | 201 `{ data: Comment }` |
| DELETE | `/comments/{commentId}` | | 204 (author only) |
| POST | `/payments` | `{ id?, groupId?, fromUserId, toUserId, amount, currency, date?, notes? }` | 201 `{ data: Expense }` (kind `payment`) |

### Categories

| Method | Path | Body | Response |
| ------ | ---- | ---- | -------- |
| GET | `/categories` | | List of `Category`: the built-ins, then the caller's own custom categories and custom categories used by expenses the caller can see (`mine: false`), by name |
| POST | `/categories` | `{ id?, name, icon?, group? }` | 201 `{ data: Category }`; repeating a create with the same `id` returns the existing category, an `id` owned by someone else returns 409 |
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
| POST | `/ai/expense-draft` | `{ text?, groupId?, receiptId? }` | `{ data: ExpenseDraft }`; 503 `AI_DISABLED` when the host did not configure AI |

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

Rules: runs in one transaction and sends no emails. People with an email resolve to the existing (or a newly invited) user, so their server id may differ from the local id: use the returned `people` map to rewrite local references. People without an email become placeholders that keep the local id. The caller becomes friends with everyone imported. Payments use `kind: "payment"` with one payer and one split (the recipient). Custom categories are created first with their device ids, so expenses can use them; deleted ones are created too and deleted once the expenses are in. A group's `defaultSplit` has its person ids remapped like `memberIds` and must match the group's members (send it only while it still applies). Validation errors name the failing item in `field`, for example `expenses[3].total`. Retrying the same import is safe.

### Maintenance

| Method | Path | Notes |
| ------ | ---- | ----- |
| POST | `/internal/jobs` | Outside `/api/v1`. Runs recurring expenses and cleanup once. Requires `Authorization: Bearer <JOBS_SECRET>`; only mounted when `JOBS_SECRET` is set. For serverless schedulers |

## MCP

The MCP server speaks Streamable HTTP at `/mcp` on the same host (send the same `Authorization: Bearer <api token>` header) and stdio via the `splitspends-mcp` binary. See [MCP.md](MCP.md).
