# Splitwise Study and Feature Parity

Split Spends aims to cover everything people use Splitwise for, with no paid tier: every feature below is available to every user of an instance.

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

| Capability | Splitwise | Split Spends |
| ---------- | --------- | ----------- |
| Email login without passwords | Password or social | Email one-time code |
| Add people by email before they join | Yes | Yes, they keep their history on first login |
| Groups with types, emoji and colour | Yes | Yes, about 200 icons in categories, or any emoji you type |
| Profile pictures | Yes, uploaded | Initials with a colour, or your Gravatar picture if you opt in |
| Simplify debts per group | Yes | Yes (greedy minimum transfers, per currency) |
| Non-group expenses with friends | Yes | Yes |
| Multiple payers | Yes | Yes |
| Split equally, exact, percent, shares, adjustment | Yes | Yes |
| Default split per group (for example rent 60/40) | Pro | Yes, equal, percent or shares; set in group settings, used by new expenses on the server, on the device and in MCP |
| Itemized split with tax and tip | Pro | Yes |
| Receipt photo attachment | Yes | Yes |
| AI receipt scanning | Pro | Yes, when the host configures an AI provider |
| Natural-language "add expense" | No | Yes, when AI is configured |
| Multiple currencies | Yes, conversion is Pro | Yes. Each group has one currency and you have your own for everything else, so nobody picks a currency per expense. AI converts receipts from abroad at that day's rate |
| Settle up and record payments | Yes | Yes |
| Remind someone to settle up | Yes | Yes, by email and push, at most once a day per person and group; share a text reminder in on-device mode |
| Invite to a group with a link or QR code | Yes | Yes, links last 14 days and also let new people sign up |
| Comments on expenses | Yes | Yes |
| Activity feed | Yes | Yes |
| Email notifications with preferences | Yes | Yes |
| Push notifications | Yes (through Google and Apple) | Web app and Android (Web Push and UnifiedPush, no Google account needed), per-kind preferences for email and push. iPhone app not yet |
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
| Server administration | No | Administrators and managers: open, invite, approval or closed sign-up, an allowed list of emails and domains, approvals, roles and disabled accounts |

## Roadmap Ideas

- Push notifications in the iPhone app (needs Apple Push Notification service keys).
- Payment links (UPI, PayPal, Venmo) on settle-up.
