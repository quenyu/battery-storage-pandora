package api

import (
	"context"
	"database/sql"
	"net/http"
)

func replaceCredential(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	id := r.PathValue("employee_id")
	var lockedID string
	// Serialize replacements without blocking FK KEY SHARE locks held by commands.
	if err := tx.QueryRowContext(ctx, `SELECT id FROM employees WHERE id=$1 FOR NO KEY UPDATE`, id).Scan(&lockedID); err != nil {
		return nil, err
	}
	var c Credential
	if err := tx.QueryRowContext(ctx, `SELECT id,employee_id,barcode,created_at,revoked_at FROM employee_credentials WHERE employee_id=$1 AND revoked_at IS NULL FOR UPDATE`, id).Scan(&c.ID, &c.EmployeeID, &c.Barcode, &c.CreatedAt, &c.RevokedAt); err != nil {
		return nil, err
	}
	if c.Barcode == in.str("barcode") {
		c.CreatedAt = c.CreatedAt.UTC()
		return c, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE employee_credentials SET revoked_at=clock_timestamp() WHERE id=$1`, c.ID); err != nil {
		return nil, err
	}
	c.ID = newID()
	c.Barcode = in.str("barcode")
	if err := tx.QueryRowContext(ctx, `INSERT INTO employee_credentials(id,employee_id,barcode) VALUES($1,$2,$3) RETURNING created_at`, c.ID, id, c.Barcode).Scan(&c.CreatedAt); err != nil {
		return nil, err
	}
	c.CreatedAt = c.CreatedAt.UTC()
	return c, nil
}

func createCabinet(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	c := Cabinet{ID: newID(), Number: in["number"].(int32)}
	_, err := tx.ExecContext(ctx, `INSERT INTO cabinets(id,number) VALUES($1,$2)`, c.ID, c.Number)
	return c, err
}
func createShelf(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	s := Shelf{ID: newID(), CabinetID: r.PathValue("cabinet_id"), Number: in["number"].(int32)}
	_, err := tx.ExecContext(ctx, `INSERT INTO shelves(id,cabinet_id,number) VALUES($1,$2,$3)`, s.ID, s.CabinetID, s.Number)
	return s, err
}
func createCell(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	id := newID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO cells(id,shelf_id,number) VALUES($1,$2,$3)`, id, r.PathValue("shelf_id"), in["number"]); err != nil {
		return nil, err
	}
	return scanCell(tx.QueryRowContext(ctx, cellSelect+` WHERE c.id=$1`, id))
}

const cellSelect = `SELECT c.id,c.shelf_id,c.number,concat(ca.number,'.',s.number,'.',c.number),b.id FROM cells c JOIN shelves s ON s.id=c.shelf_id JOIN cabinets ca ON ca.id=s.cabinet_id LEFT JOIN batteries b ON b.cell_id=c.id`

func scanCabinet(row scanner) (Cabinet, error) {
	var c Cabinet
	err := row.Scan(&c.ID, &c.Number)
	return c, err
}
func scanShelf(row scanner) (Shelf, error) {
	var s Shelf
	err := row.Scan(&s.ID, &s.CabinetID, &s.Number)
	return s, err
}
func scanCell(row scanner) (Cell, error) {
	var c Cell
	err := row.Scan(&c.ID, &c.ShelfID, &c.Number, &c.Address, &c.BatteryID)
	return c, err
}
