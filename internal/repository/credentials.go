package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
)

func (tx *Tx) GetCredential(ctx context.Context, employeeID, credentialID string) (model.Credential, error) {
	return scanCredential(tx.tx.QueryRowContext(ctx, credentialSelect+` WHERE c.employee_id = $1 AND c.id = $2`, employeeID, credentialID))
}

func (tx *Tx) FindCredential(ctx context.Context, value string) (model.Credential, error) {
	return scanCredential(tx.tx.QueryRowContext(ctx, credentialSelect+`
        WHERE c.value = $1
        ORDER BY c.disabled_at NULLS FIRST, c.created_at DESC, c.id DESC LIMIT 1
    `, value))
}

func (tx *Tx) CredentialForShare(ctx context.Context, employeeID, credentialID string) (model.Credential, error) {
	return scanCredential(tx.tx.QueryRowContext(ctx, credentialSelect+`
        WHERE c.employee_id = $1 AND c.id = $2 FOR SHARE
    `, employeeID, credentialID))
}

func (tx *Tx) LockCredentialValue(ctx context.Context, value string) error {
	_, err := tx.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 913407))`, value)
	return err
}

func (tx *Tx) CredentialUsage(ctx context.Context, value, employeeID string) (active, usedByAnotherEmployee bool, err error) {
	err = tx.tx.QueryRowContext(ctx, `
        SELECT
            EXISTS (SELECT 1 FROM employee_credentials WHERE value = $1 AND disabled_at IS NULL),
            EXISTS (SELECT 1 FROM employee_credentials WHERE value = $1 AND employee_id <> $2)
    `, value, employeeID).Scan(&active, &usedByAnotherEmployee)
	return
}

func (tx *Tx) InsertCredential(ctx context.Context, id, employeeID, value string) error {
	_, err := tx.tx.ExecContext(ctx, `
        INSERT INTO employee_credentials (id, employee_id, value) VALUES ($1, $2, $3)
    `, id, employeeID, value)
	return err
}

func (tx *Tx) DisableCredential(ctx context.Context, employeeID, credentialID string) error {
	_, err := tx.tx.ExecContext(ctx, `
        UPDATE employee_credentials
        SET disabled_at = clock_timestamp(), updated_at = clock_timestamp()
        WHERE employee_id = $1 AND id = $2 AND disabled_at IS NULL
    `, employeeID, credentialID)
	return err
}

func (r *Repository) GetEmployeeAndCredentialByValue(ctx context.Context, value string) (model.Employee, model.Credential, error) {
	query := `
        SELECT e.id, e.personnel_number, e.display_name, e.created_at, e.updated_at, e.disabled_at,
               c.id, c.employee_id, c.value, c.created_at, c.updated_at, c.disabled_at
        FROM employee_credentials c
        JOIN employees e ON e.id = c.employee_id
        WHERE c.value = $1
        ORDER BY c.disabled_at NULLS FIRST, c.created_at DESC, c.id DESC
        LIMIT 1
    `
	var employee model.Employee
	var credential model.Credential
	err := r.db.QueryRowContext(ctx, query, value).Scan(
		&employee.ID, &employee.PersonnelNumber, &employee.DisplayName,
		&employee.CreatedAt, &employee.UpdatedAt, &employee.DisabledAt,
		&credential.ID, &credential.EmployeeID, &credential.Value,
		&credential.CreatedAt, &credential.UpdatedAt, &credential.DisabledAt,
	)
	employee.IsActive = employee.DisabledAt == nil
	employee.CreatedAt, employee.UpdatedAt = employee.CreatedAt.UTC(), employee.UpdatedAt.UTC()
	if employee.DisabledAt != nil {
		disabledAt := employee.DisabledAt.UTC()
		employee.DisabledAt = &disabledAt
	}
	credential.IsActive = credential.DisabledAt == nil
	credential.CreatedAt, credential.UpdatedAt = credential.CreatedAt.UTC(), credential.UpdatedAt.UTC()
	if credential.DisabledAt != nil {
		disabledAt := credential.DisabledAt.UTC()
		credential.DisabledAt = &disabledAt
	}
	return employee, credential, err
}
