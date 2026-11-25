-- +goose Up
-- Each budget records when its window ends, so the jobs can remove budgets
-- whose window is over without knowing how long each kind of window is. Every
-- window was a day long when this column was added.
ALTER TABLE quotas ADD COLUMN ends_at timestamptz;
UPDATE quotas SET ends_at = started_at + interval '1 day';
ALTER TABLE quotas ALTER COLUMN ends_at SET NOT NULL;
CREATE INDEX quotas_ends_idx ON quotas (ends_at);

-- +goose Down
ALTER TABLE quotas DROP COLUMN ends_at;
