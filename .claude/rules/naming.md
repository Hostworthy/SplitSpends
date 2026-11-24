# Naming

Applies to new code and code you touch. Never rename contract-bound names (JSON fields, DB columns, env vars, route params).

- Do not use `From` or `Of` as connectors in function names: `toUser(u)`, not `userFromEntity(u)`.
- Keep names to 1 to 3 words and let the package or type carry context: `group.Service.Create`, not `GroupService.CreateGroup`.
- No type suffixes (`usersSlice`, `configMap`). Whole words, no invented abbreviations.
