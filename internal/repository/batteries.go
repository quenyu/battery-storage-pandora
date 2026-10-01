package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
)

func (r *Repository) GetBatteryByID(ctx context.Context, id string) (model.Battery, error) {
	return scanBattery(r.db.QueryRowContext(ctx, batterySelect+` WHERE b.id = $1`, id))
}

func (tx *Tx) GetBattery(ctx context.Context, id string) (model.Battery, error) {
	return scanBattery(tx.tx.QueryRowContext(ctx, batterySelect+` WHERE b.id = $1`, id))
}

func (tx *Tx) LockBattery(ctx context.Context, id string) (model.Battery, error) {
	return scanBattery(tx.tx.QueryRowContext(ctx, batterySelect+` WHERE b.id = $1 FOR UPDATE`, id))
}

func (tx *Tx) InsertBattery(ctx context.Context, battery model.Battery) error {
	_, err := tx.tx.ExecContext(ctx, `
        INSERT INTO batteries (id, inventory_code, serial_number, status, current_location, version)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, battery.ID, battery.InventoryCode, battery.SerialNumber, battery.Status,
		battery.CurrentLocation, battery.Version)
	return err
}

func (tx *Tx) UpdateBatteryState(ctx context.Context, operation model.Operation) error {
	_, err := tx.tx.ExecContext(ctx, `
        UPDATE batteries
        SET status = $2, current_location = $3, current_holder_employee_id = $4, version = $5
        WHERE id = $1
    `, operation.BatteryID, operation.DestinationStatus, operation.DestinationLocation,
		operation.DestinationHolderEmployeeID, operation.BatteryVersion)
	return err
}
