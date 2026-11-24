-- +goose Up
-- Devices that receive Web Push notifications: browsers through their push
-- service, Android phones through a UnifiedPush distributor. The endpoint is
-- unique: registering it again refreshes the row (and moves it to whoever is
-- signed in on that device now).
CREATE TABLE push_subscriptions (
    id              uuid PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    last_success_at timestamptz,
    failures        integer NOT NULL DEFAULT 0,
    platform        text NOT NULL CHECK (platform IN ('web', 'unifiedpush')),
    endpoint        text NOT NULL,
    p256dh          text NOT NULL,
    auth            text NOT NULL,
    label           text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX push_subscriptions_endpoint_key ON push_subscriptions (endpoint);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id, created_at DESC);

-- The VAPID key pair (RFC 8292) generated on first start when the
-- VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY variables are not set. One row, so
-- every instance and every restart signs with the same key; browsers bind
-- their subscriptions to it.
CREATE TABLE vapid_keys (
    id          smallint PRIMARY KEY CHECK (id = 1),
    created_at  timestamptz NOT NULL DEFAULT now(),
    public_key  text NOT NULL,
    private_key text NOT NULL
);

-- +goose Down
DROP TABLE vapid_keys;
DROP TABLE push_subscriptions;
