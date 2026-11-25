-- +goose Up
-- Budgets that refill every window, such as how many people one account may
-- invite a day. One row per budget: started_at is when its current window
-- began and used how much of it is gone. A single INSERT ... ON CONFLICT
-- both checks and spends the budget, so concurrent requests cannot overspend.
CREATE TABLE quotas (
    key        text PRIMARY KEY,
    started_at timestamptz NOT NULL,
    used       integer NOT NULL
);

-- +goose Down
DROP TABLE quotas;
