-- +goose Up
-- People added by name only are counted per creator.
CREATE INDEX users_placeholder_creator_idx ON users (created_by) WHERE status = 'placeholder';

-- +goose Down
DROP INDEX users_placeholder_creator_idx;
