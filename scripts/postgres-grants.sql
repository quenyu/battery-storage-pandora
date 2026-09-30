-- Run as pandora_owner in each database after applying migrations.
-- Explicit table lists ensure future tables receive no accidental privileges.
BEGIN;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO pandora_app;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM pandora_app;
GRANT SELECT, INSERT, UPDATE ON
    employees, employee_credentials, cabinets, shelves, cells, batteries,
    idempotency_records TO pandora_app;
GRANT SELECT, INSERT ON operations TO pandora_app;
COMMIT;

