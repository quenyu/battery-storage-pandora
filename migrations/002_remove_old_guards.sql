-- Upgrade existing databases without recreating tables or removing data.
ALTER TABLE batteries DROP CONSTRAINT IF EXISTS battery_position_ck;
ALTER TABLE battery_operations DROP CONSTRAINT IF EXISTS operation_shape_ck;

DROP TRIGGER IF EXISTS batteries_projection_guard ON batteries;
DROP TRIGGER IF EXISTS operations_projection_guard ON battery_operations;
DROP TRIGGER IF EXISTS requests_complete_guard ON idempotency_requests;
DROP TRIGGER IF EXISTS employees_identity ON employees;
DROP TRIGGER IF EXISTS credentials_identity ON employee_credentials;
DROP TRIGGER IF EXISTS batteries_identity ON batteries;
DROP TRIGGER IF EXISTS requests_immutable ON idempotency_requests;
DROP TRIGGER IF EXISTS requests_no_truncate ON idempotency_requests;
DROP TRIGGER IF EXISTS operations_immutable ON battery_operations;
DROP TRIGGER IF EXISTS operations_no_truncate ON battery_operations;
