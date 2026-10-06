package repository

import (
	"battery-storage-pandora/internal/model"
	"time"
)

const employeeSelect = `
    SELECT e.id, e.personnel_number, e.display_name,
           e.created_at, e.updated_at, e.disabled_at
    FROM employees e
`

func scanEmployee(row scanner) (model.Employee, error) {
	var employee model.Employee
	err := row.Scan(&employee.ID, &employee.PersonnelNumber, &employee.DisplayName,
		&employee.CreatedAt, &employee.UpdatedAt, &employee.DisabledAt)
	employee.IsActive = employee.DisabledAt == nil
	employee.CreatedAt, employee.UpdatedAt = employee.CreatedAt.UTC(), employee.UpdatedAt.UTC()
	employee.DisabledAt = utcPointer(employee.DisabledAt)
	return employee, err
}

const credentialSelect = `
    SELECT c.id, c.employee_id, c.value, c.created_at, c.disabled_at
    FROM employee_credentials c
`

func scanCredential(row scanner) (model.Credential, error) {
	var credential model.Credential
	err := row.Scan(&credential.ID, &credential.EmployeeID, &credential.Value,
		&credential.CreatedAt, &credential.DisabledAt)
	credential.IsActive = credential.DisabledAt == nil
	credential.CreatedAt = credential.CreatedAt.UTC()
	credential.DisabledAt = utcPointer(credential.DisabledAt)
	return credential, err
}

// Current battery state is the latest operation: TAKE means issued to the
// owner of its credential, any other type means stored at its location.
const batterySelect = `
    SELECT b.id, b.inventory_code, b.serial_number, b.created_at,
           l.type, l.location, c.employee_id
    FROM batteries b
    JOIN LATERAL (
        SELECT o.type, o.location, o.credential_id
        FROM battery_operations o
        WHERE o.battery_id = b.id
        ORDER BY o.id DESC
        LIMIT 1
    ) l ON true
    JOIN employee_credentials c ON c.id = l.credential_id
`

func scanBattery(row scanner) (model.Battery, error) {
	var battery model.Battery
	var lastType string
	var lastActorID int64
	err := row.Scan(&battery.ID, &battery.InventoryCode, &battery.SerialNumber, &battery.CreatedAt,
		&lastType, &battery.CurrentLocation, &lastActorID)
	battery.CreatedAt = battery.CreatedAt.UTC()
	battery.Status = model.BatteryStored
	if lastType == model.OperationTake {
		battery.Status = model.BatteryIssued
		battery.CurrentHolderEmployeeID = &lastActorID
	}
	return battery, err
}

// The source location comes from the previous operation and is computed before
// any filter is applied, so filtered history still shows where a battery came from.
const operationSelect = `
    SELECT o.id, o.battery_id, o.type, o.actor_employee_id, o.credential_id,
           o.source_location, o.destination_location, o.occurred_at
    FROM (
        SELECT o.id, o.battery_id, o.type, c.employee_id AS actor_employee_id, o.credential_id,
               lag(o.location) OVER (PARTITION BY o.battery_id ORDER BY o.id) AS source_location,
               o.location AS destination_location, o.occurred_at
        FROM battery_operations o
        JOIN employee_credentials c ON c.id = o.credential_id
    ) o
`

func scanOperation(row scanner) (model.Operation, error) {
	var operation model.Operation
	err := row.Scan(&operation.ID, &operation.BatteryID, &operation.Type,
		&operation.ActorEmployeeID, &operation.CredentialID,
		&operation.SourceLocation, &operation.DestinationLocation, &operation.OccurredAt)
	operation.OccurredAt = operation.OccurredAt.UTC()
	return operation, err
}

func utcPointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}
