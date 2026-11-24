-- +goose Up
-- People added by name only (no email yet), for example contacts imported
-- from local-only use. created_by records who may attach an email later.
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ADD COLUMN created_by uuid REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'invited', 'placeholder'));
ALTER TABLE users ADD CONSTRAINT users_email_required CHECK (status = 'placeholder' OR email IS NOT NULL);

-- Receipt images for deployments without a filesystem or object store.
CREATE TABLE blobs (
    created_at timestamptz NOT NULL DEFAULT now(),
    key        text PRIMARY KEY,
    data       bytea NOT NULL
);

-- +goose Down
DROP TABLE blobs;
DELETE FROM users WHERE status = 'placeholder';
ALTER TABLE users DROP CONSTRAINT users_email_required;
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'invited'));
ALTER TABLE users DROP COLUMN created_by;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
