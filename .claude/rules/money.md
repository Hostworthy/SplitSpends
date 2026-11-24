---
paths:
  - "backend/**/*.go"
  - "app/lib/**/*.dart"
---
# Money

- Amounts are integers in minor units (`int64` in Go, `int` in Dart) together with a currency code.
- Never parse, store or compute money with `float64` or `double`. Convert user input with exact decimal parsing (`expense.ParseDecimal`, `big.Rat`, or integer string handling in Dart).
- Use `money.Allocate` / `money.Even` whenever a total is divided; they guarantee the parts sum to the total.
- Balances are per currency. Do not add amounts of different currencies.
