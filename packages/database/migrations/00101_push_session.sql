-- +goose Up
-- A device belongs to the session or API token that registered it and is
-- removed with it, so it stops getting the account's notifications once that
-- sign-in ends (sign-out, revocation or expiry). Devices registered before
-- this have no session and stay until removed.
ALTER TABLE push_subscriptions ADD COLUMN session_id uuid REFERENCES sessions (id) ON DELETE CASCADE;
CREATE INDEX push_subscriptions_session_idx ON push_subscriptions (session_id);

-- +goose Down
ALTER TABLE push_subscriptions DROP COLUMN session_id;
