package api

import "context"

const employeeSelect = `SELECT e.id,e.personnel_number,e.display_name,e.created_at,e.updated_at,e.disabled_at FROM employees e`

func scanEmployee(row scanner) (Employee, error) {
	var e Employee
	err := row.Scan(&e.ID, &e.PersonnelNumber, &e.DisplayName, &e.CreatedAt, &e.UpdatedAt, &e.DisabledAt)
	e.IsActive = e.DisabledAt == nil
	e.CreatedAt, e.UpdatedAt = e.CreatedAt.UTC(), e.UpdatedAt.UTC()
	if e.DisabledAt != nil {
		t := e.DisabledAt.UTC()
		e.DisabledAt = &t
	}
	return e, err
}

func getEmployee(ctx context.Context, q queryer, id string) (Employee, error) {
	return scanEmployee(q.QueryRowContext(ctx, employeeSelect+` WHERE e.id=$1`, id))
}

const credentialSelect = `SELECT c.id,c.employee_id,c.value,c.created_at,c.updated_at,c.disabled_at FROM employee_credentials c`

func scanCredential(row scanner) (Credential, error) {
	var c Credential
	err := row.Scan(&c.ID, &c.EmployeeID, &c.Value, &c.CreatedAt, &c.UpdatedAt, &c.DisabledAt)
	c.IsActive = c.DisabledAt == nil
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	if c.DisabledAt != nil {
		t := c.DisabledAt.UTC()
		c.DisabledAt = &t
	}
	return c, err
}

func getCredential(ctx context.Context, q queryer, employeeID, credentialID string) (Credential, error) {
	return scanCredential(q.QueryRowContext(ctx, credentialSelect+` WHERE c.employee_id=$1 AND c.id=$2`, employeeID, credentialID))
}

const batterySelect = `SELECT b.id,b.inventory_code,b.serial_number,b.status,b.current_location,b.current_holder_employee_id,b.version,b.created_at,b.updated_at FROM batteries b`

func scanBattery(row scanner) (Battery, error) {
	var b Battery
	err := row.Scan(&b.ID, &b.InventoryCode, &b.SerialNumber, &b.Status, &b.CurrentLocation, &b.CurrentHolderEmployeeID, &b.Version, &b.CreatedAt, &b.UpdatedAt)
	b.CreatedAt, b.UpdatedAt = b.CreatedAt.UTC(), b.UpdatedAt.UTC()
	return b, err
}

func getBattery(ctx context.Context, q queryer, id string) (Battery, error) {
	return scanBattery(q.QueryRowContext(ctx, batterySelect+` WHERE b.id=$1`, id))
}

const operationSelect = `SELECT o.id,o.battery_id,o.battery_version,o.type,o.actor_employee_id,o.credential_id,o.source_status,o.destination_status,o.source_location,o.destination_location,o.source_holder_employee_id,o.destination_holder_employee_id,o.device_code,o.occurred_at FROM battery_operations o`

func scanOperation(row scanner) (Operation, error) {
	var o Operation
	err := row.Scan(&o.ID, &o.BatteryID, &o.BatteryVersion, &o.Type, &o.ActorEmployeeID, &o.CredentialID, &o.SourceStatus, &o.DestinationStatus, &o.SourceLocation, &o.DestinationLocation, &o.SourceHolderEmployeeID, &o.DestinationHolderEmployeeID, &o.DeviceCode, &o.OccurredAt)
	o.OccurredAt = o.OccurredAt.UTC()
	return o, err
}

func getOperation(ctx context.Context, q queryer, id string) (Operation, error) {
	return scanOperation(q.QueryRowContext(ctx, operationSelect+` WHERE o.id=$1`, id))
}
