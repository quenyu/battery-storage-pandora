CREATE UNIQUE INDEX credentials_one_active_per_employee_uq
    ON employee_credentials(employee_id) WHERE disabled_at IS NULL;
