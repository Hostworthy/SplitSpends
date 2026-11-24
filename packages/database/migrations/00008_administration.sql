-- +goose Up
-- Roles, disabled accounts, sign-up settings changed at runtime, and requests
-- waiting for approval.

ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'manager', 'admin'));
-- A disabled account keeps only a SHA-256 hash of its email so the same
-- address is recognised again without storing it.
ALTER TABLE users ADD COLUMN email_hash text;
CREATE UNIQUE INDEX users_email_hash_key ON users (email_hash) WHERE email_hash IS NOT NULL;
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'invited', 'placeholder', 'disabled'));
ALTER TABLE users DROP CONSTRAINT users_email_required;
ALTER TABLE users ADD CONSTRAINT users_email_required
    CHECK (status IN ('placeholder', 'disabled') OR email IS NOT NULL OR email_hash IS NOT NULL);

-- A login code sent to someone who still needs approval: verifying it files
-- the request instead of signing in.
ALTER TABLE login_challenges ADD COLUMN pending boolean NOT NULL DEFAULT false;

-- The first account was the owner until now: it becomes the administrator.
UPDATE users SET role = 'admin'
WHERE id = (SELECT id FROM users WHERE status = 'active' ORDER BY created_at, id LIMIT 1);

-- One row of sign-up settings; absent until an administrator changes them,
-- so the SIGNUP_MODE environment variable stays the starting point.
CREATE TABLE signup_settings (
    updated_by uuid REFERENCES users (id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL,
    id         boolean PRIMARY KEY DEFAULT true CHECK (id),
    mode       text NOT NULL CHECK (mode IN ('open', 'invite', 'approval', 'closed')),
    allowed    jsonb NOT NULL DEFAULT '[]'::jsonb
);

-- People who proved their email and wait for an administrator or manager.
CREATE TABLE signup_requests (
    id           uuid PRIMARY KEY,
    decided_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    requested_at timestamptz NOT NULL,
    decided_at   timestamptz,
    status       text NOT NULL CHECK (status IN ('pending', 'approved', 'declined')),
    email        text NOT NULL,
    name         text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX signup_requests_email_key ON signup_requests (lower(email));
CREATE INDEX signup_requests_status_idx ON signup_requests (status, requested_at);

-- +goose Down
ALTER TABLE login_challenges DROP COLUMN pending;
DROP TABLE signup_requests;
DROP TABLE signup_settings;
DELETE FROM users WHERE status = 'disabled' AND email IS NULL AND email_hash IS NOT NULL;
ALTER TABLE users DROP CONSTRAINT users_email_required;
ALTER TABLE users ADD CONSTRAINT users_email_required CHECK (status = 'placeholder' OR email IS NOT NULL);
UPDATE users SET status = 'invited' WHERE status = 'disabled';
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'invited', 'placeholder'));
DROP INDEX users_email_hash_key;
ALTER TABLE users DROP COLUMN email_hash;
ALTER TABLE users DROP COLUMN role;
