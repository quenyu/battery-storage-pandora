CREATE TABLE IF NOT EXISTS employees (
    id uuid PRIMARY KEY,
    personnel_number text UNIQUE,
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    disabled_at timestamptz
);

CREATE TABLE IF NOT EXISTS employee_credentials (
    id uuid PRIMARY KEY,
    employee_id uuid NOT NULL REFERENCES employees(id),
    value text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    disabled_at timestamptz,
    UNIQUE (id, employee_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS credentials_active_value_uq
    ON employee_credentials(value) WHERE disabled_at IS NULL;

CREATE TABLE IF NOT EXISTS batteries (
    id uuid PRIMARY KEY,
    inventory_code text NOT NULL UNIQUE,
    serial_number text,
    status text NOT NULL,
    current_location text,
    current_holder_employee_id uuid REFERENCES employees(id),
    version bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE UNIQUE INDEX IF NOT EXISTS batteries_one_per_location_uq
    ON batteries(current_location) WHERE current_location IS NOT NULL;

CREATE TABLE IF NOT EXISTS idempotency_requests (
    scope text NOT NULL,
    key uuid NOT NULL,
    request_hash text NOT NULL,
    http_status smallint,
    response_body jsonb,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (scope, key)
);

CREATE TABLE IF NOT EXISTS battery_operations (
    id uuid PRIMARY KEY,
    battery_id uuid NOT NULL REFERENCES batteries(id),
    battery_version bigint NOT NULL,
    type text NOT NULL,
    actor_employee_id uuid NOT NULL REFERENCES employees(id),
    credential_id uuid NOT NULL,
    source_status text,
    destination_status text NOT NULL,
    source_location text,
    destination_location text,
    source_holder_employee_id uuid REFERENCES employees(id),
    destination_holder_employee_id uuid REFERENCES employees(id),
    device_code text,
    request_scope text NOT NULL,
    request_key uuid NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (battery_id, battery_version),
    UNIQUE (request_scope, request_key),
    FOREIGN KEY (credential_id, actor_employee_id)
        REFERENCES employee_credentials(id, employee_id),
    FOREIGN KEY (request_scope, request_key)
        REFERENCES idempotency_requests(scope, key)
);
