package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
)

func (r *Repository) GetOperationByID(ctx context.Context, id string) (model.Operation, error) {
	return scanOperation(r.db.QueryRowContext(ctx, operationSelect+` WHERE o.id = $1`, id))
}

func (tx *Tx) RecordOperation(ctx context.Context, operation *model.Operation) error {
	err := tx.tx.QueryRowContext(ctx, `
        INSERT INTO battery_operations (
            id, battery_id, battery_version, type, actor_employee_id, credential_id,
            source_status, destination_status, source_location, destination_location,
            source_holder_employee_id, destination_holder_employee_id
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
        RETURNING occurred_at
    `, operation.ID, operation.BatteryID, operation.BatteryVersion, operation.Type,
		operation.ActorEmployeeID, operation.CredentialID, operation.SourceStatus, operation.DestinationStatus,
		operation.SourceLocation, operation.DestinationLocation, operation.SourceHolderEmployeeID,
		operation.DestinationHolderEmployeeID).Scan(&operation.OccurredAt)
	if err != nil {
		return err
	}
	operation.OccurredAt = operation.OccurredAt.UTC()

	_, err = tx.tx.ExecContext(ctx, `UPDATE batteries SET updated_at = $2 WHERE id = $1`,
		operation.BatteryID, operation.OccurredAt)
	return err
}
