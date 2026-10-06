package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"time"
)

func (r *Repository) GetEmployeeByID(ctx context.Context, id int64) (model.Employee, error) {
	return scanEmployee(r.db.QueryRowContext(ctx, employeeSelect+` WHERE e.id = $1`, id))
}

func (tx *Tx) GetEmployee(ctx context.Context, id int64) (model.Employee, error) {
	return scanEmployee(tx.tx.QueryRowContext(ctx, employeeSelect+` WHERE e.id = $1`, id))
}

func (tx *Tx) LockEmployee(ctx context.Context, id int64) (model.Employee, error) {
	return scanEmployee(tx.tx.QueryRowContext(ctx, employeeSelect+` WHERE e.id = $1 FOR UPDATE`, id))
}

func (tx *Tx) EmployeeForShare(ctx context.Context, id int64) (model.Employee, error) {
	return scanEmployee(tx.tx.QueryRowContext(ctx, employeeSelect+` WHERE e.id = $1 FOR SHARE`, id))
}

func (tx *Tx) InsertEmployee(ctx context.Context, input model.CreateEmployeeInput) (int64, error) {
	var id int64
	err := tx.tx.QueryRowContext(ctx, `
        INSERT INTO employees (display_name, personnel_number) VALUES ($1, $2) RETURNING id
    `, input.DisplayName, input.PersonnelNumber).Scan(&id)
	return id, err
}

func (tx *Tx) UpdateEmployee(ctx context.Context, employee model.Employee) error {
	_, err := tx.tx.ExecContext(ctx, `
        UPDATE employees
        SET display_name = $2, disabled_at = $3, updated_at = clock_timestamp()
        WHERE id = $1
    `, employee.ID, employee.DisplayName, employee.DisabledAt)
	return err
}

func (tx *Tx) HasCustody(ctx context.Context, employeeID int64) (bool, error) {
	var hasBatteries bool
	err := tx.tx.QueryRowContext(ctx, `SELECT EXISTS (`+batterySelect+`
        WHERE l.type = 'TAKE' AND c.employee_id = $1)
    `, employeeID).Scan(&hasBatteries)
	return hasBatteries, err
}

func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	var now time.Time
	err := tx.tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now.UTC(), err
}
