package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

const batterySelect = `SELECT b.id,b.inventory_code,b.status,b.cell_id,b.holder_employee_id,b.version,o.id,b.created_at,b.updated_at FROM batteries b JOIN operations o ON o.battery_id=b.id AND o.battery_version=b.version`

func scanBattery(row scanner) (Battery, error) {
	var b Battery
	err := row.Scan(&b.ID, &b.InventoryCode, &b.Status, &b.CellID, &b.HolderEmployeeID, &b.Version, &b.LastOperationID, &b.CreatedAt, &b.UpdatedAt)
	b.CreatedAt = b.CreatedAt.UTC()
	b.UpdatedAt = b.UpdatedAt.UTC()
	return b, err
}

func getBattery(ctx context.Context, q queryer, id string) (Battery, error) {
	return scanBattery(q.QueryRowContext(ctx, batterySelect+` WHERE b.id=$1`, id))
}

const operationSelect = `SELECT o.id,o.battery_id,o.battery_version,o.type,o.actor_employee_id,o.credential_id,o.from_cell_id,o.to_cell_id,o.from_holder_employee_id,o.to_holder_employee_id,o.reason,o.occurred_at FROM operations o`

func scanOperation(row scanner) (Operation, error) {
	var o Operation
	err := row.Scan(&o.ID, &o.BatteryID, &o.BatteryVersion, &o.Type, &o.ActorEmployeeID, &o.CredentialID, &o.FromCellID, &o.ToCellID, &o.FromHolderEmployeeID, &o.ToHolderEmployeeID, &o.Reason, &o.OccurredAt)
	o.OccurredAt = o.OccurredAt.UTC()
	return o, err
}

func getOperation(ctx context.Context, q queryer, id string) (Operation, error) {
	return scanOperation(q.QueryRowContext(ctx, operationSelect+` WHERE o.id=$1`, id))
}

// Lock the credential until commit so replacement cannot revoke it midway
// through a command. Idempotent replay has already been handled by the caller.
func commandActor(ctx context.Context, tx *sql.Tx, barcode string) (employeeID, credentialID string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT employee_id,id FROM employee_credentials WHERE barcode=$1 AND revoked_at IS NULL FOR SHARE`, barcode).Scan(&employeeID, &credentialID)
	if errors.Is(err, sql.ErrNoRows) {
		err = fail(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "Действующий штрихкод сотрудника не найден")
	}
	return
}

func requireCell(ctx context.Context, tx *sql.Tx, id string) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cells WHERE id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return notFound()
	}
	return nil
}

func registerBattery(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	employeeID, credentialID, err := commandActor(ctx, tx, in.str("employee_barcode"))
	if err != nil {
		return nil, err
	}
	cellID := in.str("cell_id")
	if err = requireCell(ctx, tx, cellID); err != nil {
		return nil, err
	}
	id := newID()
	// The unique cell index arbitrates competing placements; a preflight
	// occupancy check would race and would not replace this constraint.
	_, err = tx.ExecContext(ctx, `INSERT INTO batteries(id,inventory_code,status,cell_id,version) VALUES($1,$2,'stored',$3,1)`, id, in.str("inventory_code"), cellID)
	if err != nil {
		return nil, err
	}
	operation := Operation{
		ID: newID(), BatteryID: id, BatteryVersion: 1, Type: "register",
		ActorEmployeeID: employeeID, CredentialID: credentialID, ToCellID: &cellID,
	}
	return finishBatteryCommand(ctx, tx, operation)
}

func batteryCommand(kind string) command {
	return func(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
		employeeID, credentialID, err := commandActor(ctx, tx, in.str("employee_barcode"))
		if err != nil {
			return nil, err
		}
		// Read only the locked battery row here. Joining its operation before
		// waiting could couple the new row version to an older query snapshot.
		battery, err := scanBattery(tx.QueryRowContext(ctx, `SELECT b.id,b.inventory_code,b.status,b.cell_id,b.holder_employee_id,b.version,''::text,b.created_at,b.updated_at FROM batteries b WHERE b.id=$1 FOR UPDATE`, r.PathValue("battery_id")))
		if err != nil {
			return nil, err
		}
		operation := Operation{
			ID: newID(), BatteryID: battery.ID, BatteryVersion: battery.Version + 1, Type: kind,
			ActorEmployeeID: employeeID, CredentialID: credentialID,
			FromCellID: battery.CellID, FromHolderEmployeeID: battery.HolderEmployeeID,
		}
		var status string
		switch kind {
		case "checkout":
			if battery.Status != "stored" {
				return nil, conflict("INVALID_TRANSITION")
			}
			status = "issued"
			operation.ToHolderEmployeeID = &employeeID
		case "return", "move":
			expected := "issued"
			if kind == "move" {
				expected = "stored"
			}
			target := in.str("target_cell_id")
			if battery.Status != expected || (battery.CellID != nil && *battery.CellID == target) {
				return nil, conflict("INVALID_TRANSITION")
			}
			if err = requireCell(ctx, tx, target); err != nil {
				return nil, err
			}
			status = "stored"
			operation.ToCellID = &target
		case "loss":
			if battery.Status != "stored" && battery.Status != "issued" {
				return nil, conflict("INVALID_TRANSITION")
			}
			status = "lost"
			reason := in.str("reason")
			operation.Reason = &reason
		default:
			return nil, fmt.Errorf("unsupported battery command: %q", kind)
		}
		_, err = tx.ExecContext(ctx, `UPDATE batteries SET status=$2,cell_id=$3,holder_employee_id=$4,version=$5 WHERE id=$1`, battery.ID, status, operation.ToCellID, operation.ToHolderEmployeeID, operation.BatteryVersion)
		if err != nil {
			return nil, err
		}
		return finishBatteryCommand(ctx, tx, operation)
	}
}

func finishBatteryCommand(ctx context.Context, tx *sql.Tx, operation Operation) (CommandResult, error) {
	var result CommandResult
	// The DB clock is sampled at insertion after all state/index locks. Both
	// state and journal use exactly the same timestamp inside this transaction.
	err := tx.QueryRowContext(ctx, `INSERT INTO operations(id,battery_id,battery_version,type,actor_employee_id,credential_id,from_cell_id,to_cell_id,from_holder_employee_id,to_holder_employee_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING occurred_at`, operation.ID, operation.BatteryID, operation.BatteryVersion, operation.Type, operation.ActorEmployeeID, operation.CredentialID, operation.FromCellID, operation.ToCellID, operation.FromHolderEmployeeID, operation.ToHolderEmployeeID, operation.Reason).Scan(&operation.OccurredAt)
	if err != nil {
		return result, err
	}
	operation.OccurredAt = operation.OccurredAt.UTC()
	if _, err = tx.ExecContext(ctx, `UPDATE batteries SET updated_at=$2 WHERE id=$1`, operation.BatteryID, operation.OccurredAt); err != nil {
		return result, err
	}
	result.Operation = operation
	result.Battery, err = getBattery(ctx, tx, operation.BatteryID)
	return result, err
}
