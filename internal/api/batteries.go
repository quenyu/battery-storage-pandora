package api

import (
	"context"
	"database/sql"
	"net/http"
)

func registerBattery(ctx context.Context, tx *sql.Tx, request *http.Request, values input) (any, error) {
	employeeID, credentialID, err := commandActor(ctx, tx, values.str("actor_credential_value"))
	if err != nil {
		return nil, err
	}
	batteryID := newID()
	destination := values.str("destination_location")
	_, err = tx.ExecContext(ctx, `
        INSERT INTO batteries (id, inventory_code, serial_number, status, current_location, version)
        VALUES ($1, $2, $3, 'STORED', $4, 1)
    `, batteryID, values.str("inventory_code"), values["serial_number"], destination)
	if err != nil {
		return nil, err
	}
	operation := Operation{
		ID:                  newID(),
		BatteryID:           batteryID,
		BatteryVersion:      1,
		Type:                "STORE",
		ActorEmployeeID:     employeeID,
		CredentialID:        credentialID,
		DestinationStatus:   "STORED",
		DestinationLocation: &destination,
	}
	return finishBatteryCommand(ctx, tx, request.Header.Get("Idempotency-Key"), operation)
}

func batteryCommand(kind string) command {
	return func(ctx context.Context, tx *sql.Tx, request *http.Request, values input) (any, error) {
		employeeID, credentialID, err := commandActor(ctx, tx, values.str("actor_credential_value"))
		if err != nil {
			return nil, err
		}
		battery, err := scanBattery(tx.QueryRowContext(ctx,
			batterySelect+` WHERE b.id = $1 FOR UPDATE`, request.PathValue("battery_id")))
		if err != nil {
			return nil, err
		}
		if expected, supplied := values["expected_version"]; supplied && expected.(int64) != battery.Version {
			return nil, conflict("STATE_VERSION_MISMATCH")
		}
		if observed, supplied := values["observed_source_location"]; supplied {
			if battery.CurrentLocation == nil || *battery.CurrentLocation != observed.(string) {
				return nil, conflict("SOURCE_MISMATCH")
			}
		}
		operation := Operation{
			ID:                     newID(),
			BatteryID:              battery.ID,
			BatteryVersion:         battery.Version + 1,
			Type:                   kind,
			ActorEmployeeID:        employeeID,
			CredentialID:           credentialID,
			SourceStatus:           &battery.Status,
			SourceLocation:         battery.CurrentLocation,
			SourceHolderEmployeeID: battery.CurrentHolderEmployeeID,
		}
		destination := values.str("destination_location")
		switch kind {
		case "TAKE":
			if battery.Status == "ISSUED" {
				return nil, conflict("BATTERY_ALREADY_ISSUED")
			}
			if battery.Status != "STORED" {
				return nil, conflict("INVALID_BATTERY_STATE")
			}
			operation.DestinationStatus = "ISSUED"
			operation.DestinationHolderEmployeeID = &employeeID
		case "RETURN":
			if battery.Status != "ISSUED" {
				return nil, conflict("INVALID_BATTERY_STATE")
			}
			if battery.CurrentHolderEmployeeID == nil || *battery.CurrentHolderEmployeeID != employeeID {
				return nil, fail(403, "RETURN_NOT_ALLOWED", "Вернуть АКБ может только взявший сотрудник")
			}
			operation.DestinationStatus = "STORED"
			operation.DestinationLocation = &destination
		case "MOVE":
			if battery.Status != "STORED" {
				return nil, conflict("INVALID_BATTERY_STATE")
			}
			if battery.CurrentLocation != nil && *battery.CurrentLocation == destination {
				return nil, conflict("SAME_LOCATION")
			}
			operation.DestinationStatus = "STORED"
			operation.DestinationLocation = &destination
		default:
			return nil, fail(500, "INTERNAL_ERROR", "Неизвестная команда")
		}

		// UNIQUE current_location resolves races between different batteries.
		_, err = tx.ExecContext(ctx, `
            UPDATE batteries
            SET status = $2, current_location = $3, current_holder_employee_id = $4, version = $5
            WHERE id = $1
        `, battery.ID, operation.DestinationStatus, operation.DestinationLocation,
			operation.DestinationHolderEmployeeID, operation.BatteryVersion)
		if err != nil {
			return nil, err
		}
		return finishBatteryCommand(ctx, tx, request.Header.Get("Idempotency-Key"), operation)
	}
}

func finishBatteryCommand(ctx context.Context, tx *sql.Tx, requestKey string, operation Operation) (CommandResult, error) {
	err := tx.QueryRowContext(ctx, `
        INSERT INTO battery_operations (
            id, battery_id, battery_version, type, actor_employee_id, credential_id,
            source_status, destination_status, source_location, destination_location,
            source_holder_employee_id, destination_holder_employee_id, request_scope, request_key
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
        RETURNING occurred_at
    `, operation.ID, operation.BatteryID, operation.BatteryVersion, operation.Type,
		operation.ActorEmployeeID, operation.CredentialID, operation.SourceStatus, operation.DestinationStatus,
		operation.SourceLocation, operation.DestinationLocation, operation.SourceHolderEmployeeID,
		operation.DestinationHolderEmployeeID, requestScope, requestKey).Scan(&operation.OccurredAt)
	if err != nil {
		return CommandResult{}, err
	}
	operation.OccurredAt = operation.OccurredAt.UTC()
	// State and history use the exact same database timestamp.
	if _, err := tx.ExecContext(ctx, `UPDATE batteries SET updated_at = $2 WHERE id = $1`,
		operation.BatteryID, operation.OccurredAt); err != nil {
		return CommandResult{}, err
	}
	battery, err := getBattery(ctx, tx, operation.BatteryID)
	if err != nil {
		return CommandResult{}, err
	}
	return CommandResult{Battery: battery, Operation: operation}, nil
}
