# Splitwise Study and Feature Parity

SplitSpends aims to cover everything people use Splitwise for, with no paid tier: every feature below is available to every user of an instance.

## What Splitwise Does

Splitwise tracks shared costs between people and tells everyone who owes whom. Its core model:

- **People and friends.** Anyone can be added by email, even before they sign up. Friend balances combine all shared groups plus expenses outside groups.
- **Groups** (home, trip, couple, other) with members, a default currency and an optional "simplify debts" setting that rewrites the settle-up plan into the fewest payments.
- **Expenses** with a description, amount, currency, date, category, notes, receipt photo and comments. One or many people can pay; the cost can be split equally, by exact amounts, by percentages, by shares, or by adjustment.
- **Payments** ("settle up") are recorded like expenses, so balances come from one ledger.
- **Activity feed** of everything that happened, plus email notifications.
- **Recurring expenses** for rent and subscriptions, **search**, **charts**, **CSV export**, and restoring deleted expenses.

Several of these sit behind Splitwise Pro today (receipt scanning, itemization, charts, search, currency conversion, unlimited daily expenses), and the free tier adds ads and daily limits.

## Parity Matrix

| Capability | Splitwise | SplitSpends |
| ---------- | --------- | ----------- |
| Email login without passwords | Password or social | Email one-time code |
| Add people by email before they join | Yes | Yes, they keep their history on first login |
| Groups with types, emoji and colour | Yes | Yes |
| Simplify debts per group | Yes | Yes (greedy minimum transfers, per currency) |
| Non-group expenses with friends | Yes | Yes |
| Multiple payers | Yes | Yes |
| Split equally, exact, percent, shares, adjustment | Yes | Yes |
| Default split per group (for example rent 60/40) | Pro | Yes, equal, percent or shares; set in group settings, used by new expenses on the server, on the device and in MCP |
| Itemized split with tax and tip | Pro | Yes |
| Receipt photo attachment | Yes | Yes |
| AI receipt scanning | Pro | Yes, when the host configures an AI provider |
| Natural-language "add expense" | No | Yes, when AI is configured |
| Multiple currencies | Yes, conversion is Pro | Yes, balances tracked per currency (no conversion yet) |
| Settle up and record payments | Yes | Yes |
| Comments on expenses | Yes | Yes |
| Activity feed | Yes | Yes |
| Email notifications with preferences | Yes | Yes |
| Recurring expenses | Yes | Weekly, every two weeks, monthly, yearly |
| Search and filters | Pro | Yes |
| Charts by category, month and member | Pro | Yes |
| Custom categories | No | Yes, with a name, icon and group; shared expenses show them to other members |
| CSV export | Yes | Yes |
| Restore deleted expenses | Yes | Yes (undo snackbar and restore) |
| Daily expense limits or ads | Free tier | Never |
| API for third parties | Yes | Yes, plus personal API tokens |
| AI assistant integration | No | MCP server (HTTP and stdio) |
| Self-hosting | No | Yes, one container plus PostgreSQL |

## Roadmap Ideas

- Currency conversion with stored exchange rates at expense time.
- Push notifications for the mobile app.
- Payment links (UPI, PayPal, Venmo) on settle-up.
- Offline queue for adding expenses without connectivity.
- S3-compatible receipt storage behind the existing `BlobStore` port.
