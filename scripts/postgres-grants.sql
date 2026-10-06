-- Run as pandora_owner in each database after applying migrations.
-- Explicit table and column lists ensure future tables and columns receive no accidental privileges.
-- The application may update only what it changes: employee name/activity and card deactivation.
-- Batteries and battery_operations are insert-only, so recorded history cannot change retroactively.
BEGIN;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO pandora_app;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM pandora_app;
GRANT SELECT, INSERT ON employees, employee_credentials, batteries, battery_operations TO pandora_app;
GRANT UPDATE (display_name, disabled_at, updated_at) ON employees TO pandora_app;
GRANT UPDATE (disabled_at) ON employee_credentials TO pandora_app;
COMMIT;
