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

Completed: migration002, all19 REST routes, atomic current/history/result, credentials/custody,
literal search and confirmed numeric addresses, filter-bound cursors, isolated launch/demo and README.
User confirmed addresses are three positive integers without leading zeroes; test employees are allowed.
Still open: employee source and enterprise authentication/permissions. Corporate integrations are absent.

Evidence: full PostgreSQL suite on owner + restricted app role PASS, OpenAPI19/20/187 PASS,
go vet and build PASS, Chrome local Swagger Try it out create/replay/read/credential/STORE PASS,
PowerShell STORE→TAKE→replacement→RETURN→MOVE→replay PASS.
Independent runtime review found one literal inventory-code lookup mismatch; fixed and regression-tested.
Race check unavailable: CGO/C compiler absent. Docker/production backup-restore not exercised.
Original pandora counts unchanged: employees5, batteries0, operations0, requests5.

Commits: 98542df migration; e76b505 API + tests; 091f2aa isolated launch/docs.
Evidence files: .local/mvp-go-test-results.json, .local/browser-evidence-mvp/results.json and screenshots.
Local server uses pandora_mvp_demo at127.0.0.1:18080, explicit development adapter.
No remaining MVP implementation tasks; enterprise decisions and incompatible legacy-data policy remain separate.
