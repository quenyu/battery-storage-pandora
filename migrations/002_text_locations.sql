-- Forward-only conversion of 001. Never edit 001 or rewrite historical actors.
-- The migration runner owns one transaction (including preflight and backfill).
-- Stop the old API before deployment: its contract cannot use the new tables.
BEGIN;

LOCK TABLE employees, employee_credentials, cabinets, shelves, cells,
    batteries, operations, idempotency_records IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM batteries WHERE status = 'lost')
        OR EXISTS (SELECT 1 FROM operations WHERE type = 'loss') THEN
        RAISE EXCEPTION 'migration 002 blocked: legacy LOST/loss has no agreed MVP equivalent; retain data and resolve policy before migrating';
    END IF;
    IF EXISTS (SELECT 1 FROM operations WHERE type = 'return'
        AND actor_employee_id IS DISTINCT FROM from_holder_employee_id) THEN
        RAISE EXCEPTION 'migration 002 blocked: legacy RETURN actor differs from holder; history must not be rewritten';
    END IF;
    IF EXISTS (
        SELECT 1 FROM batteries b
        LEFT JOIN LATERAL (SELECT count(*) AS n, min(battery_version) AS first_version,
            max(battery_version) AS last_version FROM operations WHERE battery_id=b.id) stats ON true
        LEFT JOIN operations last ON last.battery_id=b.id AND last.battery_version=b.version
        WHERE stats.n <> b.version OR stats.first_version IS DISTINCT FROM 1
            OR stats.last_version IS DISTINCT FROM b.version OR last.id IS NULL
            OR b.status IS DISTINCT FROM CASE WHEN last.type='checkout' THEN 'issued' ELSE 'stored' END
            OR b.cell_id IS DISTINCT FROM last.to_cell_id
            OR b.holder_employee_id IS DISTINCT FROM last.to_holder_employee_id
            OR b.updated_at IS DISTINCT FROM last.occurred_at
    ) THEN
        RAISE EXCEPTION 'migration 002 blocked: legacy battery projection/version/time does not match complete history';
    END IF;
    IF EXISTS (
        SELECT 1 FROM (
            SELECT o.*, lag(to_cell_id) OVER history AS previous_cell,
                lag(to_holder_employee_id) OVER history AS previous_holder,
                lag(type) OVER history AS previous_type
            FROM operations o WINDOW history AS (PARTITION BY battery_id ORDER BY battery_version)
        ) chain
        WHERE (battery_version=1 AND type <> 'register')
            OR (battery_version>1 AND (type='register'
                OR from_cell_id IS DISTINCT FROM previous_cell
                OR from_holder_employee_id IS DISTINCT FROM previous_holder
                OR (type='return' AND previous_type <> 'checkout')
                OR (type IN ('checkout','move') AND previous_type NOT IN ('register','return','move'))))
    ) THEN
        RAISE EXCEPTION 'migration 002 blocked: legacy operation sources/types do not form a valid version chain';
    END IF;
END;
$$;

-- Keep every original row, FK, timestamp and old replay result in this schema.
-- Rename ALL indexes as well: PostgreSQL index names are schema-wide, and new
-- primary/unique indexes would otherwise collide with the archived table names.
DO $$
DECLARE
    legacy_table text;
    old_index record;
BEGIN
    FOREACH legacy_table IN ARRAY ARRAY['employees','employee_credentials','cabinets',
        'shelves','cells','batteries','operations','idempotency_records'] LOOP
        FOR old_index IN
            SELECT n.nspname, i.relname FROM pg_index x
            JOIN pg_class i ON i.oid=x.indexrelid JOIN pg_namespace n ON n.oid=i.relnamespace
            WHERE x.indrelid=to_regclass(format('%I.%I',current_schema(),legacy_table))
        LOOP
            EXECUTE format('ALTER INDEX %I.%I RENAME TO %I',
                old_index.nspname, old_index.relname, 'legacy_' || old_index.relname);
        END LOOP;
        EXECUTE format('ALTER TABLE %I.%I RENAME TO %I',current_schema(),legacy_table,'legacy_' || legacy_table);
    END LOOP;
END;
$$;

-- Runtime DDL follows the agreed design package. Legacy tables are an archive,
-- not a location registry used by the new backend.
CREATE TABLE employees (
    id uuid PRIMARY KEY,
    personnel_number text UNIQUE CHECK (personnel_number IS NULL OR length(btrim(personnel_number)) > 0),
    display_name text NOT NULL CHECK (length(btrim(display_name)) > 0),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    disabled_at timestamptz,
    CHECK (disabled_at IS NULL OR disabled_at >= created_at)
);

CREATE TABLE employee_credentials (
    id uuid PRIMARY KEY,
    employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    value text COLLATE "C" NOT NULL CHECK (length(btrim(value)) > 0),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    disabled_at timestamptz,
    UNIQUE (id, employee_id),
    CHECK (disabled_at IS NULL OR disabled_at >= created_at)
);
CREATE UNIQUE INDEX credentials_active_value_uq
    ON employee_credentials(value) WHERE disabled_at IS NULL;
CREATE INDEX credentials_employee_idx ON employee_credentials(employee_id, created_at, id);
-- No unique(employee_id): several active credentials are deliberately supported.

CREATE TABLE batteries (
    id uuid PRIMARY KEY,
    inventory_code text COLLATE "C" NOT NULL UNIQUE CHECK (length(btrim(inventory_code)) > 0),
    serial_number text CHECK (serial_number IS NULL OR length(btrim(serial_number)) > 0),
    status text NOT NULL CHECK (status IN ('STORED', 'ISSUED')),
    current_location text COLLATE "C" CHECK (current_location IS NULL OR current_location ~ '^[1-9][0-9]*\.[1-9][0-9]*\.[1-9][0-9]*$'),
    current_holder_employee_id uuid REFERENCES employees(id) ON DELETE RESTRICT,
    version bigint NOT NULL CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT battery_position_ck CHECK (
        (status = 'STORED' AND current_location IS NOT NULL AND current_holder_employee_id IS NULL)
        OR (status = 'ISSUED' AND current_location IS NULL AND current_holder_employee_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX batteries_one_per_location_uq ON batteries(current_location) WHERE current_location IS NOT NULL;
CREATE INDEX batteries_holder_idx ON batteries(current_holder_employee_id, id) WHERE current_holder_employee_id IS NOT NULL;
CREATE INDEX batteries_status_idx ON batteries(status, id);
-- serial_number not UNIQUE: uniqueness scope/presence has not been confirmed.

CREATE TABLE idempotency_requests (
    scope text COLLATE "C" NOT NULL CHECK (length(btrim(scope)) > 0),
    key uuid NOT NULL,
    request_hash char(64) NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    http_status smallint CHECK ((http_status BETWEEN 200 AND 299) OR http_status IN (404,409,422)),
    response_body jsonb,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (scope, key),
    CHECK ((http_status IS NULL AND response_body IS NULL)
        OR (http_status IS NOT NULL AND response_body IS NOT NULL))
);

CREATE TABLE battery_operations (
    id uuid PRIMARY KEY,
    battery_id uuid NOT NULL REFERENCES batteries(id) ON DELETE RESTRICT,
    battery_version bigint NOT NULL CHECK (battery_version > 0),
    type text NOT NULL CHECK (type IN ('STORE','TAKE','RETURN','MOVE')),
    actor_employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    credential_id uuid NOT NULL,
    source_status text CHECK (source_status IS NULL OR source_status IN ('STORED','ISSUED')),
    destination_status text NOT NULL CHECK (destination_status IN ('STORED','ISSUED')),
    source_location text COLLATE "C" CHECK (source_location IS NULL OR source_location ~ '^[1-9][0-9]*\.[1-9][0-9]*\.[1-9][0-9]*$'),
    destination_location text COLLATE "C" CHECK (destination_location IS NULL OR destination_location ~ '^[1-9][0-9]*\.[1-9][0-9]*\.[1-9][0-9]*$'),
    source_holder_employee_id uuid REFERENCES employees(id) ON DELETE RESTRICT,
    destination_holder_employee_id uuid REFERENCES employees(id) ON DELETE RESTRICT,
    device_code text COLLATE "C" CHECK (device_code IS NULL OR length(btrim(device_code)) > 0),
    request_scope text NOT NULL,
    request_key uuid NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (battery_id, battery_version),
    UNIQUE (request_scope, request_key),
    FOREIGN KEY (credential_id, actor_employee_id)
        REFERENCES employee_credentials(id, employee_id) ON DELETE RESTRICT,
    FOREIGN KEY (request_scope, request_key)
        REFERENCES idempotency_requests(scope, key) ON DELETE RESTRICT,
    CONSTRAINT operation_shape_ck CHECK (
        (type = 'STORE' AND battery_version = 1 AND source_status IS NULL AND destination_status = 'STORED'
            AND source_location IS NULL AND destination_location IS NOT NULL
            AND source_holder_employee_id IS NULL AND destination_holder_employee_id IS NULL)
        OR (type = 'TAKE' AND battery_version > 1 AND source_status IS NOT NULL AND source_status = 'STORED'
            AND destination_status = 'ISSUED' AND source_location IS NOT NULL AND destination_location IS NULL
            AND source_holder_employee_id IS NULL AND destination_holder_employee_id IS NOT NULL
            AND destination_holder_employee_id = actor_employee_id)
        OR (type = 'RETURN' AND battery_version > 1 AND source_status IS NOT NULL AND source_status = 'ISSUED'
            AND destination_status = 'STORED' AND source_location IS NULL AND destination_location IS NOT NULL
            AND source_holder_employee_id IS NOT NULL AND destination_holder_employee_id IS NULL
            AND source_holder_employee_id = actor_employee_id)
        OR (type = 'MOVE' AND battery_version > 1 AND source_status IS NOT NULL AND source_status = 'STORED'
            AND destination_status = 'STORED' AND source_location IS NOT NULL AND destination_location IS NOT NULL
            AND source_location <> destination_location
            AND source_holder_employee_id IS NULL AND destination_holder_employee_id IS NULL)
    )
);
-- UNIQUE(battery_id,battery_version) also supports backward history scans.
CREATE INDEX operations_time_idx ON battery_operations(occurred_at DESC, id DESC);
CREATE INDEX operations_actor_idx ON battery_operations(actor_employee_id, occurred_at DESC, id DESC);
CREATE INDEX operations_source_holder_idx ON battery_operations(source_holder_employee_id, occurred_at DESC, id DESC)
    WHERE source_holder_employee_id IS NOT NULL;
CREATE INDEX operations_destination_holder_idx ON battery_operations(destination_holder_employee_id, occurred_at DESC, id DESC)
    WHERE destination_holder_employee_id IS NOT NULL;
CREATE INDEX operations_source_location_idx ON battery_operations(source_location, occurred_at DESC, id DESC)
    WHERE source_location IS NOT NULL;
CREATE INDEX operations_destination_location_idx ON battery_operations(destination_location, occurred_at DESC, id DESC)
    WHERE destination_location IS NOT NULL;
CREATE INDEX operations_device_idx ON battery_operations(device_code, occurred_at DESC, id DESC) WHERE device_code IS NOT NULL;
CREATE INDEX operations_credential_idx ON battery_operations(credential_id);

-- Defense against application mistakes. Owner/superuser is a separate trust boundary.
CREATE FUNCTION reject_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'history is append-only' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER operations_immutable BEFORE UPDATE OR DELETE ON battery_operations
    FOR EACH ROW EXECUTE FUNCTION reject_history_mutation();
CREATE TRIGGER operations_no_truncate BEFORE TRUNCATE ON battery_operations
    FOR EACH STATEMENT EXECUTE FUNCTION reject_history_mutation();

CREATE FUNCTION protect_stable_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id THEN
        RAISE EXCEPTION 'id is immutable' USING ERRCODE = '55000';
    END IF;
    CASE TG_TABLE_NAME
        WHEN 'employee_credentials' THEN
            IF NEW.value IS DISTINCT FROM OLD.value OR NEW.employee_id IS DISTINCT FROM OLD.employee_id THEN
                RAISE EXCEPTION 'credential identity is immutable' USING ERRCODE='55000'; END IF;
        WHEN 'batteries' THEN
            IF NEW.inventory_code IS DISTINCT FROM OLD.inventory_code THEN
                RAISE EXCEPTION 'inventory identity is immutable' USING ERRCODE='55000'; END IF;
        ELSE NULL;
    END CASE;
    RETURN NEW;
END;
$$;
CREATE TRIGGER employees_identity BEFORE UPDATE ON employees FOR EACH ROW EXECUTE FUNCTION protect_stable_identity();
CREATE TRIGGER credentials_identity BEFORE UPDATE ON employee_credentials FOR EACH ROW EXECUTE FUNCTION protect_stable_identity();
CREATE TRIGGER batteries_identity BEFORE UPDATE ON batteries FOR EACH ROW EXECUTE FUNCTION protect_stable_identity();

-- Deferred so event and current can be written in either order within one transaction.
CREATE FUNCTION check_battery_projection() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_id uuid;
    current_row batteries%ROWTYPE;
    last_event battery_operations%ROWTYPE;
    previous_event battery_operations%ROWTYPE;
BEGIN
    IF TG_TABLE_NAME = 'batteries' THEN target_id := NEW.id;
    ELSE target_id := NEW.battery_id;
    END IF;
    SELECT * INTO current_row FROM batteries WHERE id = target_id;
    SELECT * INTO last_event FROM battery_operations WHERE battery_id = target_id
        ORDER BY battery_version DESC LIMIT 1;
    IF current_row.id IS NULL OR last_event.id IS NULL
        OR current_row.version IS DISTINCT FROM last_event.battery_version
        OR current_row.status IS DISTINCT FROM last_event.destination_status
        OR current_row.current_location IS DISTINCT FROM last_event.destination_location
        OR current_row.current_holder_employee_id IS DISTINCT FROM last_event.destination_holder_employee_id
        OR current_row.updated_at IS DISTINCT FROM last_event.occurred_at THEN
        RAISE EXCEPTION 'current state must match latest operation' USING ERRCODE='23514';
    END IF;
    IF TG_TABLE_NAME = 'battery_operations' THEN
      IF NEW.battery_version > 1 THEN
        SELECT * INTO previous_event FROM battery_operations
            WHERE battery_id = target_id AND battery_version = NEW.battery_version - 1;
        IF previous_event.id IS NULL
            OR NEW.source_status IS DISTINCT FROM previous_event.destination_status
            OR NEW.source_location IS DISTINCT FROM previous_event.destination_location
            OR NEW.source_holder_employee_id IS DISTINCT FROM previous_event.destination_holder_employee_id THEN
            RAISE EXCEPTION 'operation source must match previous version' USING ERRCODE='23514';
        END IF;
      END IF;
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER batteries_projection_guard AFTER INSERT OR UPDATE ON batteries
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_battery_projection();
CREATE CONSTRAINT TRIGGER operations_projection_guard AFTER INSERT ON battery_operations
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_battery_projection();

CREATE FUNCTION check_request_completed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM idempotency_requests WHERE scope=NEW.scope AND key=NEW.key
        AND (http_status IS NULL OR response_body IS NULL)) THEN
        RAISE EXCEPTION 'request cannot commit without result' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER requests_complete_guard AFTER INSERT OR UPDATE ON idempotency_requests
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_request_completed();
CREATE FUNCTION protect_request_result() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN RAISE EXCEPTION 'request retention is not configured' USING ERRCODE='55000'; END IF;
    IF OLD.http_status IS NOT NULL OR NEW.scope IS DISTINCT FROM OLD.scope OR NEW.key IS DISTINCT FROM OLD.key
        OR NEW.request_hash IS DISTINCT FROM OLD.request_hash OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'request identity and completed result are immutable' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER requests_immutable BEFORE UPDATE OR DELETE ON idempotency_requests
    FOR EACH ROW EXECUTE FUNCTION protect_request_result();
CREATE TRIGGER requests_no_truncate BEFORE TRUNCATE ON idempotency_requests
    FOR EACH STATEMENT EXECUTE FUNCTION reject_history_mutation();

INSERT INTO employees(id,display_name,created_at,updated_at)
    SELECT id,name,created_at,created_at FROM legacy_employees;
INSERT INTO employee_credentials(id,employee_id,value,created_at,updated_at,disabled_at)
    SELECT id,employee_id,barcode,created_at,COALESCE(revoked_at,created_at),revoked_at
    FROM legacy_employee_credentials;

-- Matches the old API cellSelect exactly: decimal cabinet.shelf.cell numbers.
-- The agreed format is three positive decimal integers, without leading zeroes.
INSERT INTO batteries(id,inventory_code,status,current_location,current_holder_employee_id,
    version,created_at,updated_at)
SELECT b.id,b.inventory_code,upper(b.status),
    CASE WHEN b.cell_id IS NOT NULL THEN concat(ca.number,'.',s.number,'.',c.number) END,
    b.holder_employee_id,b.version,b.created_at,b.updated_at
FROM legacy_batteries b
LEFT JOIN legacy_cells c ON c.id=b.cell_id
LEFT JOIN legacy_shelves s ON s.id=c.shelf_id
LEFT JOIN legacy_cabinets ca ON ca.id=s.cabinet_id;

-- Old keys were global strings with old route/body hashes and old JSON. Their
-- replay semantics cannot be safely translated into a principal/UUID namespace.
-- Preserve the originals exclusively in legacy_idempotency_records. Each copied
-- event gets a completed, reserved migration-only provenance record instead.
INSERT INTO idempotency_requests(scope,key,request_hash,http_status,response_body,created_at)
SELECT 'legacy-migration',id,repeat('0',64),201,
    jsonb_build_object('migration',jsonb_build_object('operation_id',id,
        'legacy_api_replay_supported',false)),occurred_at
FROM legacy_operations;

INSERT INTO battery_operations(id,battery_id,battery_version,type,actor_employee_id,
    credential_id,source_status,destination_status,source_location,destination_location,
    source_holder_employee_id,destination_holder_employee_id,request_scope,request_key,occurred_at)
SELECT o.id,o.battery_id,o.battery_version,
    CASE o.type WHEN 'register' THEN 'STORE' WHEN 'checkout' THEN 'TAKE'
        WHEN 'return' THEN 'RETURN' WHEN 'move' THEN 'MOVE' END,
    o.actor_employee_id,o.credential_id,
    CASE o.type WHEN 'register' THEN NULL WHEN 'return' THEN 'ISSUED' ELSE 'STORED' END,
    CASE o.type WHEN 'checkout' THEN 'ISSUED' ELSE 'STORED' END,
    CASE WHEN o.from_cell_id IS NOT NULL THEN concat(source_ca.number,'.',source_s.number,'.',source_c.number) END,
    CASE WHEN o.to_cell_id IS NOT NULL THEN concat(destination_ca.number,'.',destination_s.number,'.',destination_c.number) END,
    o.from_holder_employee_id,o.to_holder_employee_id,'legacy-migration',o.id,o.occurred_at
FROM legacy_operations o
LEFT JOIN legacy_cells source_c ON source_c.id=o.from_cell_id
LEFT JOIN legacy_shelves source_s ON source_s.id=source_c.shelf_id
LEFT JOIN legacy_cabinets source_ca ON source_ca.id=source_s.cabinet_id
LEFT JOIN legacy_cells destination_c ON destination_c.id=o.to_cell_id
LEFT JOIN legacy_shelves destination_s ON destination_s.id=destination_c.shelf_id
LEFT JOIN legacy_cabinets destination_ca ON destination_ca.id=destination_s.cabinet_id
ORDER BY o.battery_id,o.battery_version;

-- Renaming preserves previous grants. Freeze the archive so an old runtime
-- credential cannot mutate historical data through those inherited grants.
DO $$
DECLARE legacy_table text;
BEGIN
    FOREACH legacy_table IN ARRAY ARRAY['legacy_employees','legacy_employee_credentials',
        'legacy_cabinets','legacy_shelves','legacy_cells','legacy_batteries',
        'legacy_operations','legacy_idempotency_records'] LOOP
        EXECUTE format('CREATE TRIGGER legacy_archive_immutable BEFORE INSERT OR UPDATE OR DELETE ON %I.%I FOR EACH ROW EXECUTE FUNCTION reject_history_mutation()',current_schema(),legacy_table);
        EXECUTE format('CREATE TRIGGER legacy_archive_no_truncate BEFORE TRUNCATE ON %I.%I FOR EACH STATEMENT EXECUTE FUNCTION reject_history_mutation()',current_schema(),legacy_table);
    END LOOP;
END;
$$;

-- Validate deferred history/projection/result guards before recording success.
SET CONSTRAINTS ALL IMMEDIATE;
COMMIT;


