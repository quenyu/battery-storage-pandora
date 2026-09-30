# MVP implementation status

Baseline: `a49c668`, existing demonstration preserved on `main`.
Implementation branch: `feature/text-location-mvp`.
No AGENTS.md was present. Initial tests pass; PostgreSQL integration tests initially skipped (no TEST_DATABASE_URL).

Task boundaries:
- Root: HTTP input/auth/errors, transactional commands, launch/docs, integration and commits.
- Migration agent: additive 002 migration, legacy-data preservation and migration tests.
- Reads agent: response models, SQL reads and filter-bound keyset cursors.
- Integration agent: PostgreSQL HTTP/contract tests and browser smoke script.

Stable contract: discovery OpenAPI/schema; pgx database/sql adapter retained, explicit SQL.
Old location tables/data retained under legacy_* names, new runtime has no location registry.
Incompatible old LOSS or third-party RETURN must abort migration without changing data.
Old numeric addresses retain their existing rendered cabinet.shelf.cell text, without claiming it is the enterprise canonical format.

Pending business decisions: address canonicalization, employee source, enterprise authentication/permissions.
No corporate integration or address alias policy will be invented. Local development adapter is explicit and separate.

Next: implement commands, integrate migration/reads/tests, run real PostgreSQL tests and Swagger Try it out in browser.
