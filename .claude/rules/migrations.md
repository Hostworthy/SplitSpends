---
paths:
  - "packages/database/migrations/**"
---
# Migrations

- Never edit a migration that may have been applied. Add `000NN_description.sql` with `-- +goose Up` and `-- +goose Down` sections.
- Order columns fixed-width first (uuid, timestamptz, bigint, integer, boolean), variable-width last (text, jsonb, arrays) to reduce row padding.
- Money columns are `bigint` minor units.
