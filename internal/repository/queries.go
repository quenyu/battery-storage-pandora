package repository

import (
	"battery-storage-pandora/internal/model"
	"time"
)

const userSelect = `
    SELECT u.id, u.barcode, u.name,
           u.created_at, u.updated_at, u.disabled_at
    FROM users u
`

func scanUser(row scanner) (model.User, error) {
	var user model.User
	err := row.Scan(&user.ID, &user.Barcode, &user.Name,
		&user.CreatedAt, &user.UpdatedAt, &user.DisabledAt)
	user.CreatedAt, user.UpdatedAt = user.CreatedAt.UTC(), user.UpdatedAt.UTC()
	user.DisabledAt = utcPointer(user.DisabledAt)
	return user, err
}

// Current battery state is the latest operation: TAKE means issued to its
// user, any other type means stored at its location.
const batterySelect = `
    SELECT b.id, b.inventory_code, b.created_at,
           l.type, l.location, l.user_id
    FROM batteries b
    JOIN LATERAL (
        SELECT o.type, o.location, o.user_id
        FROM battery_operations o
        WHERE o.battery_id = b.id
        ORDER BY o.id DESC
        LIMIT 1
    ) l ON true
`

func scanBattery(row scanner) (model.Battery, error) {
	var battery model.Battery
	var lastType string
	var lastActorID int64
	err := row.Scan(&battery.ID, &battery.InventoryCode, &battery.CreatedAt,
		&lastType, &battery.CurrentLocation, &lastActorID)
	battery.CreatedAt = battery.CreatedAt.UTC()
	battery.Status = model.BatteryStored
	if lastType == model.OperationTake {
		battery.Status = model.BatteryIssued
		battery.CurrentHolderUserID = &lastActorID
	}
	return battery, err
}

// The source location comes from the previous operation and is computed before
// any filter is applied, so filtered history still shows where a battery came from.
const operationSelect = `
    SELECT o.id, o.battery_id, o.type, o.actor_user_id,
           o.source_location, o.destination_location, o.occurred_at
    FROM (
        SELECT o.id, o.battery_id, o.type, o.user_id AS actor_user_id,
               lag(o.location) OVER (PARTITION BY o.battery_id ORDER BY o.id) AS source_location,
               o.location AS destination_location, o.occurred_at
        FROM battery_operations o
    ) o
`

func scanOperation(row scanner) (model.Operation, error) {
	var operation model.Operation
	err := row.Scan(&operation.ID, &operation.BatteryID, &operation.Type, &operation.ActorUserID,
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
