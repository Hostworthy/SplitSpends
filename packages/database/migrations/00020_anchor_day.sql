-- +goose Up
-- The day of the month a monthly or yearly series falls on. Shorter months
-- book their last day and the series returns to this day afterwards.
ALTER TABLE expenses ADD COLUMN anchor_day smallint CHECK (anchor_day BETWEEN 1 AND 31);
-- Existing series continue on the day of the expense that carries them.
UPDATE expenses SET anchor_day = EXTRACT(DAY FROM date) WHERE recurrence <> 'none';

-- +goose Down
ALTER TABLE expenses DROP COLUMN anchor_day;
