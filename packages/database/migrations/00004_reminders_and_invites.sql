-- +goose Up
-- The last settle-up reminder per sender, recipient and scope (a group, or
-- none for an overall reminder). The unique index lets one INSERT ... ON
-- CONFLICT statement both check the 24 hour window and record the reminder,
-- so concurrent requests cannot both send.
CREATE TABLE reminders (
    id       uuid PRIMARY KEY,
    from_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    to_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    group_id uuid REFERENCES groups (id) ON DELETE CASCADE,
    sent_at  timestamptz NOT NULL
);
CREATE UNIQUE INDEX reminders_scope_key
    ON reminders (from_id, to_id, (COALESCE(group_id, '00000000-0000-0000-0000-000000000000'::uuid)));

-- Shareable group invite links. Only the SHA-256 hash of the token is kept.
CREATE TABLE group_invites (
    id         uuid PRIMARY KEY,
    group_id   uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    created_by uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    token_hash text NOT NULL
);
CREATE UNIQUE INDEX group_invites_token_hash_key ON group_invites (token_hash);
CREATE INDEX group_invites_group_idx ON group_invites (group_id, created_at DESC);

-- +goose Down
DROP TABLE group_invites;
DROP TABLE reminders;
