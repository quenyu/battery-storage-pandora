package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"time"
)

func (r *Repository) GetEmployee(ctx context.Context, id string) (model.Employee, error) {
	return getEmployee(ctx, r.db, id)
}

func (tx *Tx) GetEmployee(ctx context.Context, id string) (model.Employee, error) {
	return getEmployee(ctx, tx.tx, id)
}

func (tx *Tx) LockEmployee(ctx context.Context, id string) (model.Employee, error) {
	return scanEmployee(tx.tx.QueryRowContext(ctx, employeeSelect+` WHERE e.id = $1 FOR UPDATE`, id))
}

func (tx *Tx) EmployeeForShare(ctx context.Context, id string) (model.Employee, error) {
	return scanEmployee(tx.tx.QueryRowContext(ctx, employeeSelect+` WHERE e.id = $1 FOR SHARE`, id))
}

func (tx *Tx) InsertEmployee(ctx context.Context, id string, input model.CreateEmployeeInput) error {
	_, err := tx.tx.ExecContext(ctx, `
        INSERT INTO employees (id, display_name, personnel_number)
        VALUES ($1, $2, $3)
    `, id, input.DisplayName, input.PersonnelNumber)
	return err
}

func (tx *Tx) UpdateEmployee(ctx context.Context, employee model.Employee) error {
	_, err := tx.tx.ExecContext(ctx, `
        UPDATE employees
        SET display_name = $2, disabled_at = $3, updated_at = clock_timestamp()
        WHERE id = $1
    `, employee.ID, employee.DisplayName, employee.DisabledAt)
	return err
}

func (tx *Tx) HasCustody(ctx context.Context, employeeID string) (bool, error) {
	var hasBatteries bool
	err := tx.tx.QueryRowContext(ctx, `
        SELECT EXISTS (SELECT 1 FROM batteries WHERE current_holder_employee_id = $1)
    `, employeeID).Scan(&hasBatteries)
	return hasBatteries, err
}

func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	var now time.Time
	err := tx.tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now.UTC(), err
}
