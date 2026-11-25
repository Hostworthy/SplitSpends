-- +goose Up
-- Uploads count each person's receipts that no expense uses, and the cleanup
-- job removes old ones; both look receipts up by uploader or age and check
-- expenses by receipt.
CREATE INDEX receipts_uploader_idx ON receipts (uploaded_by);
CREATE INDEX receipts_created_idx ON receipts (created_at);
CREATE INDEX expenses_receipt_idx ON expenses (receipt_id) WHERE receipt_id IS NOT NULL;

-- +goose Down
DROP INDEX expenses_receipt_idx;
DROP INDEX receipts_created_idx;
DROP INDEX receipts_uploader_idx;
