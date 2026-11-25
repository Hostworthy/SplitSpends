---
paths:
  - "packages/database/migrations/**"
---
# Migrations

- Never edit a migration that may have been applied. Add `000NN_description.sql` with a `-- +goose Up` section only.
- Migrations only go forward: no `-- +goose Down` sections, and never run the ones older files carry. A release is undone by restoring a backup; a schema change by a new migration that reverses it.
- A new constraint must accept every row the code could already have written: add it `NOT VALID`, then validate it so that existing rows that fail leave it unvalidated with a warning instead of stopping the server from starting.
- Order columns fixed-width first (uuid, timestamptz, bigint, integer, boolean), variable-width last (text, jsonb, arrays) to reduce row padding.
- Money columns are `bigint` minor units.
