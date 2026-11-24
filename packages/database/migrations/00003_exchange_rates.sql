-- +goose Up
-- Cached exchange rates for showing totals in each person's currency.
-- Amounts and balances never change with rates; this table only feeds display.
CREATE TABLE exchange_rates (
    published_at timestamptz NOT NULL,
    fetched_at   timestamptz NOT NULL,
    base         text PRIMARY KEY,
    source       text NOT NULL,
    rates        jsonb NOT NULL
);

-- +goose Down
DROP TABLE exchange_rates;
