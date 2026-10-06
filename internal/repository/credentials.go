package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
)

// An active card wins over disabled cards with the same value.
const findCredentialQuery = credentialSelect + `
    WHERE c.value = $1
    ORDER BY c.disabled_at NULLS FIRST, c.id DESC
    LIMIT 1
`

func (r *Repository) FindCredential(ctx context.Context, value string) (model.Credential, error) {
	return scanCredential(r.db.QueryRowContext(ctx, findCredentialQuery, value))
}

func (tx *Tx) FindCredential(ctx context.Context, value string) (model.Credential, error) {
	return scanCredential(tx.tx.QueryRowContext(ctx, findCredentialQuery, value))
}

func (tx *Tx) GetCredential(ctx context.Context, employeeID, credentialID int64) (model.Credential, error) {
	return scanCredential(tx.tx.QueryRowContext(ctx, credentialSelect+` WHERE c.employee_id = $1 AND c.id = $2`, employeeID, credentialID))
}

func (tx *Tx) CredentialForShare(ctx context.Context, employeeID, credentialID int64) (model.Credential, error) {
	return scanCredential(tx.tx.QueryRowContext(ctx, credentialSelect+`
        WHERE c.employee_id = $1 AND c.id = $2 FOR SHARE
    `, employeeID, credentialID))
}

func (tx *Tx) LockCredentialValue(ctx context.Context, value string) error {
	_, err := tx.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 913407))`, value)
	return err
}

func (tx *Tx) CredentialUsage(ctx context.Context, value string, employeeID int64) (active, usedByAnotherEmployee bool, err error) {
	err = tx.tx.QueryRowContext(ctx, `
        SELECT
            EXISTS (SELECT 1 FROM employee_credentials WHERE value = $1 AND disabled_at IS NULL),
            EXISTS (SELECT 1 FROM employee_credentials WHERE value = $1 AND employee_id <> $2)
    `, value, employeeID).Scan(&active, &usedByAnotherEmployee)
	return
}

func (tx *Tx) InsertCredential(ctx context.Context, employeeID int64, value string) (int64, error) {
	var id int64
	err := tx.tx.QueryRowContext(ctx, `
        INSERT INTO employee_credentials (employee_id, value) VALUES ($1, $2) RETURNING id
    `, employeeID, value).Scan(&id)
	return id, err
}

func (tx *Tx) HasActiveCredential(ctx context.Context, employeeID int64) (bool, error) {
	var active bool
	err := tx.tx.QueryRowContext(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM employee_credentials
            WHERE employee_id = $1 AND disabled_at IS NULL
        )
    `, employeeID).Scan(&active)
	return active, err
}

func (tx *Tx) DisableCredential(ctx context.Context, employeeID, credentialID int64) error {
	_, err := tx.tx.ExecContext(ctx, `
        UPDATE employee_credentials SET disabled_at = clock_timestamp()
        WHERE employee_id = $1 AND id = $2 AND disabled_at IS NULL
    `, employeeID, credentialID)
	return err
}
