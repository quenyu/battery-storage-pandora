package repository

import (
	"context"
	"database/sql"
)

type Tx struct {
	tx *sql.Tx
}

func (r *Repository) Begin(ctx context.Context) (*Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
        SET LOCAL lock_timeout = '5s';
        SET LOCAL statement_timeout = '10s';
        SET LOCAL TIME ZONE 'UTC'
    `); err != nil {
		tx.Rollback()
		return nil, err
	}
	return &Tx{tx: tx}, nil
}

func (tx *Tx) Commit() error   { return tx.tx.Commit() }
func (tx *Tx) Rollback() error { return tx.tx.Rollback() }

func (tx *Tx) Savepoint(ctx context.Context) error {
	_, err := tx.tx.ExecContext(ctx, `SAVEPOINT business`)
	return err
}

func (tx *Tx) RollbackToSavepoint(ctx context.Context) error {
	_, err := tx.tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT business`)
	return err
}
