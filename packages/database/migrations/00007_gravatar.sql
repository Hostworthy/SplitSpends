-- +goose Up
-- People can opt in to showing their Gravatar picture instead of initials.
ALTER TABLE users ADD COLUMN gravatar boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users DROP COLUMN gravatar;
