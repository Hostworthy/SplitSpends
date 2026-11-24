-- +goose Up
-- Column order: fixed-width columns (uuid, timestamptz, bigint, int, bool)
-- first, variable-width (text, jsonb, arrays) last, to minimise row padding.

CREATE TABLE users (
    id               uuid PRIMARY KEY,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    email            text NOT NULL,
    name             text NOT NULL,
    default_currency text NOT NULL DEFAULT 'USD',
    avatar_color     text NOT NULL,
    timezone         text NOT NULL DEFAULT 'UTC',
    status           text NOT NULL CHECK (status IN ('active', 'invited')),
    notifications    jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE login_challenges (
    id          uuid PRIMARY KEY,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL,
    attempts    integer NOT NULL DEFAULT 0,
    email       text NOT NULL,
    code_hash   text NOT NULL
);
CREATE INDEX login_challenges_email_idx ON login_challenges (email, created_at DESC);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at   timestamptz,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL,
    kind         text NOT NULL CHECK (kind IN ('session', 'token')),
    name         text NOT NULL DEFAULT '',
    token_hash   text NOT NULL,
    user_agent   text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX sessions_token_hash_key ON sessions (token_hash);
CREATE INDEX sessions_user_idx ON sessions (user_id, kind);

CREATE TABLE friendships (
    user_a     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_b     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (user_a, user_b),
    CHECK (user_a < user_b)
);
CREATE INDEX friendships_user_b_idx ON friendships (user_b);

CREATE TABLE groups (
    id               uuid PRIMARY KEY,
    created_by       uuid NOT NULL REFERENCES users (id),
    archived_at      timestamptz,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    simplify_debts   boolean NOT NULL DEFAULT true,
    name             text NOT NULL,
    kind             text NOT NULL CHECK (kind IN ('home', 'trip', 'couple', 'other')),
    default_currency text NOT NULL,
    emoji            text NOT NULL DEFAULT '',
    color            text NOT NULL DEFAULT '#00897B'
);

CREATE TABLE group_members (
    group_id  uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    joined_at timestamptz NOT NULL,
    left_at   timestamptz,
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_idx ON group_members (user_id) WHERE left_at IS NULL;

CREATE TABLE receipts (
    id           uuid PRIMARY KEY,
    uploaded_by  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL,
    size         bigint NOT NULL,
    content_type text NOT NULL,
    storage_key  text NOT NULL
);

CREATE TABLE expenses (
    id          uuid PRIMARY KEY,
    group_id    uuid REFERENCES groups (id) ON DELETE CASCADE,
    receipt_id  uuid REFERENCES receipts (id) ON DELETE SET NULL,
    created_by  uuid NOT NULL REFERENCES users (id),
    updated_by  uuid NOT NULL REFERENCES users (id),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    deleted_at  timestamptz,
    total       bigint NOT NULL CHECK (total > 0),
    tax         bigint NOT NULL DEFAULT 0,
    tip         bigint NOT NULL DEFAULT 0,
    date        date NOT NULL,
    next_date   date,
    kind        text NOT NULL CHECK (kind IN ('expense', 'payment')),
    description text NOT NULL,
    category    text NOT NULL,
    currency    text NOT NULL,
    notes       text NOT NULL DEFAULT '',
    method      text NOT NULL,
    recurrence  text NOT NULL DEFAULT 'none',
    items       jsonb NOT NULL DEFAULT '[]'::jsonb
);
CREATE INDEX expenses_group_idx ON expenses (group_id, date DESC) WHERE deleted_at IS NULL;
CREATE INDEX expenses_due_idx ON expenses (next_date) WHERE deleted_at IS NULL AND recurrence <> 'none';

CREATE TABLE expense_shares (
    expense_id uuid NOT NULL REFERENCES expenses (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id),
    paid       bigint NOT NULL DEFAULT 0,
    owed       bigint NOT NULL DEFAULT 0,
    value      text NOT NULL DEFAULT '',
    PRIMARY KEY (expense_id, user_id)
);
CREATE INDEX expense_shares_user_idx ON expense_shares (user_id);

CREATE TABLE comments (
    id         uuid PRIMARY KEY,
    expense_id uuid NOT NULL REFERENCES expenses (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    body       text NOT NULL
);
CREATE INDEX comments_expense_idx ON comments (expense_id, created_at);

CREATE TABLE activities (
    id         uuid PRIMARY KEY,
    actor_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    group_id   uuid REFERENCES groups (id) ON DELETE CASCADE,
    expense_id uuid REFERENCES expenses (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    kind       text NOT NULL,
    involved   uuid[] NOT NULL,
    data       jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX activities_involved_idx ON activities USING gin (involved);
CREATE INDEX activities_created_idx ON activities (created_at DESC);
CREATE INDEX activities_group_idx ON activities (group_id, created_at DESC);

-- +goose Down
DROP TABLE activities;
DROP TABLE comments;
DROP TABLE expense_shares;
DROP TABLE expenses;
DROP TABLE receipts;
DROP TABLE group_members;
DROP TABLE groups;
DROP TABLE friendships;
DROP TABLE sessions;
DROP TABLE login_challenges;
DROP TABLE users;
