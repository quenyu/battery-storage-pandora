package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
)

// The battery_id condition lets PostgreSQL compute the history window for one battery only.
func (r *Repository) GetOperationByID(ctx context.Context, id int64) (model.Operation, error) {
	return scanOperation(r.db.QueryRowContext(ctx, operationSelect+`
        WHERE o.battery_id = (SELECT battery_id FROM battery_operations WHERE id = $1) AND o.id = $1
    `, id))
}

func (tx *Tx) LatestOperation(ctx context.Context, batteryID int64) (model.Operation, error) {
	return scanOperation(tx.tx.QueryRowContext(ctx, operationSelect+`
        WHERE o.battery_id = $1 ORDER BY o.id DESC LIMIT 1
    `, batteryID))
}

func (tx *Tx) InsertOperation(ctx context.Context, batteryID int64, kind string, userID int64, location *string) error {
	_, err := tx.tx.ExecContext(ctx, `
        INSERT INTO battery_operations (battery_id, type, user_id, location)
        VALUES ($1, $2, $3, $4)
    `, batteryID, kind, userID, location)
	return err
}
