---
name: add-domain
description: Add a feature or bounded context end to end (entity, port, service, migration, repository, contract, handler, Flutter, MCP). Use when adding features or CRUD.
argument-hint: "<Name>"
---

# Add Domain

Workflow for `$ARGUMENTS` following [ADR-001](../../../docs/adr/001-domain-driven-design.md).

```text
- [ ] 1. Entity with invariants in backend/internal/core/domains/<name>/
- [ ] 2. Repository interface next to the entity
- [ ] 3. Service using the interface; errors via errs.Validation/NotFound/Conflict
- [ ] 4. Unit tests for the rules
- [ ] 5. Migration 000NN_<name>.sql (new file; never edit applied ones)
- [ ] 6. Repository in backend/internal/database/repositories/
- [ ] 7. Wire in backend/internal/bootstrap/runtime.go (Services + Wire)
- [ ] 8. Contract types in backend/internal/platform/contract.go and docs/API.md
- [ ] 9. Handler + serializer in backend/internal/api/, route in router.go
- [ ] 10. Flutter model, repository and screens in app/lib/features/<name>/
- [ ] 11. MCP tool if assistants should use it
- [ ] 12. make check (and make test-integration when SQL changed)
```

Conventions:

- Envelopes: `{data}` for one object, `{items,total}` for lists, 204 for deletes.
- Path params use `{name}` in `http.ServeMux` patterns; parse with `pathID`.
- Authorization lives in services (membership checks), not handlers.
- Record user-visible changes in the activity feed inside the same transaction (`ports.Tx`).
