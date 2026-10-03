-- Stop the old application before applying this migration: it requires these
-- columns and the response cache. Business history and its permissions remain.
ALTER TABLE battery_operations
    DROP COLUMN request_scope,
    DROP COLUMN request_key;

DROP TABLE idempotency_requests;
