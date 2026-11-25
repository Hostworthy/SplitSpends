---
paths:
  - "packages/database/migrations/**"
---
# Migrations

- Never edit a migration that may have been applied. Add `000NN_description.sql` with a `-- +goose Up` section only.
- Migrations only go forward: no `-- +goose Down` sections, and never run the ones older files carry. A release is undone by restoring a backup; a schema change by a new migration that reverses it.
- Order columns fixed-width first (uuid, timestamptz, bigint, integer, boolean), variable-width last (text, jsonb, arrays) to reduce row padding.
- Money columns are `bigint` minor units.
