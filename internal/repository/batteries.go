package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"database/sql"
	"errors"
)

func (r *Repository) GetBatteryByID(ctx context.Context, id int64) (model.Battery, error) {
	return scanBattery(r.db.QueryRowContext(ctx, batterySelect+` WHERE b.id = $1`, id))
}

func (tx *Tx) GetBattery(ctx context.Context, id int64) (model.Battery, error) {
	return scanBattery(tx.tx.QueryRowContext(ctx, batterySelect+` WHERE b.id = $1`, id))
}

// LockBattery serializes commands on one battery until commit. Batteries are
// immutable, so an advisory lock is used instead of a row lock, which would
// require UPDATE privilege on the table.
func (tx *Tx) LockBattery(ctx context.Context, inventoryCode string) (int64, error) {
	if _, err := tx.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 471113))`, inventoryCode); err != nil {
		return 0, err
	}
	var id int64
	err := tx.tx.QueryRowContext(ctx, `SELECT id FROM batteries WHERE inventory_code = $1`, inventoryCode).Scan(&id)
	return id, err
}

// InsertBattery reports inserted=false when the inventory code already exists.
// A concurrent insert of the same code is waited for instead of failing.
func (tx *Tx) InsertBattery(ctx context.Context, inventoryCode string, serialNumber *string) (id int64, inserted bool, err error) {
	err = tx.tx.QueryRowContext(ctx, `
        INSERT INTO batteries (inventory_code, serial_number) VALUES ($1, $2)
        ON CONFLICT (inventory_code) DO NOTHING
        RETURNING id
    `, inventoryCode, serialNumber).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// LockLocation serializes, until commit, every transaction that puts a battery
// into the same location.
func (tx *Tx) LockLocation(ctx context.Context, location string) error {
	_, err := tx.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 552901))`, location)
	return err
}

// LocationOccupied checks current state only: a location is occupied when it is
// the location of some battery's latest operation.
func (tx *Tx) LocationOccupied(ctx context.Context, location string) (bool, error) {
	var occupied bool
	err := tx.tx.QueryRowContext(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM battery_operations o
            WHERE o.location = $1
              AND o.id = (SELECT max(n.id) FROM battery_operations n WHERE n.battery_id = o.battery_id)
        )
    `, location).Scan(&occupied)
	return occupied, err
}
