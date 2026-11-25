# ADR-005: Money, the Ledger and Debt Simplification

## Status

Accepted

## Context

Rounding errors and float arithmetic are the classic bugs in bill splitting. Balances must always sum to zero.

## Decision

- Amounts are `int64` minor units with the currency's ISO exponent (JPY 0, EUR 2, KWD 3). Floats never carry money: JSON carries integers, split inputs such as percentages are decimal strings parsed into exact rationals, and responses send them back as strings (`shares[].value`), so no client reads them as floats.
- Allocation uses the largest-remainder method with deterministic tie breaking, so parts always sum exactly to the total.
- Payments are expenses of kind `payment` (the payer paid, the recipient owes). Every balance comes from one ledger of `(paid, owed)` shares.
- Balances are kept per currency. There is no implicit conversion.
- Without simplification, debts are pairwise: within each expense, debtors owe creditors in proportion to what each creditor fronted, and opposite debts between two people cancel. Each debt starts at the floor of its exact share; the leftover units go out over debtors and creditors in user id order so that every debtor's debts add up to what they owe and every creditor is owed exactly what they fronted. Pairwise debts therefore always agree with the member nets, and do not depend on the order the shares are read in.
- With simplification, each currency's net balances are settled greedily, largest debtor to largest creditor, which needs at most n−1 transfers.
- Friend balances sum the debts between two people across shared groups (simplified or not, per group) plus non-group expenses, so the friend view matches the group views.
- Expenses are soft-deleted and can be restored. Members can leave a group, and friends can be removed, only when settled.

## Consequences

### Positive

- Every group balance sums to zero by construction, verified in tests.

### Negative

- Multi-currency groups show one balance per currency until conversion is added.
