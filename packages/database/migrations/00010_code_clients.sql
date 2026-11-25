-- +goose Up
-- The network a login code was requested from, as a keyed hash, so the
-- hourly limit applies per network and per email address.
ALTER TABLE login_challenges ADD COLUMN client_hash text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE login_challenges DROP COLUMN client_hash;
