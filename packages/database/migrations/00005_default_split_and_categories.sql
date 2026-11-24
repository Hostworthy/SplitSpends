-- +goose Up
-- A group's default split: {"method": "equal" | "percent" | "shares",
-- "values": {"<userId>": "<decimal>"}}. NULL when the group has none.
ALTER TABLE groups ADD COLUMN default_split jsonb;

-- User-defined expense categories (ids are custom.<uuid>). Deleted ones are
-- kept so expenses that use them keep a valid id.
CREATE TABLE categories (
    owner_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz,
    id         text PRIMARY KEY,
    name       text NOT NULL,
    icon       text NOT NULL,
    group_name text NOT NULL
);
CREATE INDEX categories_owner_idx ON categories (owner_id) WHERE deleted_at IS NULL;
CREATE INDEX expenses_custom_category_idx ON expenses (category) WHERE category LIKE 'custom.%';

-- +goose Down
DROP INDEX expenses_custom_category_idx;
DROP TABLE categories;
ALTER TABLE groups DROP COLUMN default_split;
