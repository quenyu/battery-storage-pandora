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
    SELECT c.id, c.employee_id, c.value,
           c.created_at, c.updated_at, c.disabled_at
    FROM employee_credentials c
`

func scanCredential(row scanner) (model.Credential, error) {
	var credential model.Credential
	err := row.Scan(&credential.ID, &credential.EmployeeID, &credential.Value,
		&credential.CreatedAt, &credential.UpdatedAt, &credential.DisabledAt)
	credential.IsActive = credential.DisabledAt == nil
	credential.CreatedAt, credential.UpdatedAt = credential.CreatedAt.UTC(), credential.UpdatedAt.UTC()
	credential.DisabledAt = utcPointer(credential.DisabledAt)
	return credential, err
}

const batterySelect = `
    SELECT b.id, b.inventory_code, b.serial_number, b.status,
           b.current_location, b.current_holder_employee_id,
           b.version, b.created_at, b.updated_at
    FROM batteries b
`

func scanBattery(row scanner) (model.Battery, error) {
	var battery model.Battery
	err := row.Scan(&battery.ID, &battery.InventoryCode, &battery.SerialNumber, &battery.Status,
		&battery.CurrentLocation, &battery.CurrentHolderEmployeeID, &battery.Version,
		&battery.CreatedAt, &battery.UpdatedAt)
	battery.CreatedAt, battery.UpdatedAt = battery.CreatedAt.UTC(), battery.UpdatedAt.UTC()
	return battery, err
}

const operationSelect = `
    SELECT o.id, o.battery_id, o.battery_version, o.type,
           o.actor_employee_id, o.credential_id,
           o.source_status, o.destination_status,
           o.source_location, o.destination_location,
           o.source_holder_employee_id, o.destination_holder_employee_id,
           o.occurred_at
    FROM battery_operations o
`

func scanOperation(row scanner) (model.Operation, error) {
	var operation model.Operation
	err := row.Scan(&operation.ID, &operation.BatteryID, &operation.BatteryVersion, &operation.Type,
		&operation.ActorEmployeeID, &operation.CredentialID,
		&operation.SourceStatus, &operation.DestinationStatus,
		&operation.SourceLocation, &operation.DestinationLocation,
		&operation.SourceHolderEmployeeID, &operation.DestinationHolderEmployeeID,
		&operation.OccurredAt)
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
