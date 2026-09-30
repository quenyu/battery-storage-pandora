package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

func createEmployee(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	id := newID()
	var pn any
	if v, ok := in["personnel_number"]; ok {
		pn = v
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO employees(id,display_name,personnel_number) VALUES($1,$2,$3)`, id, in.str("display_name"), pn); err != nil {
		return nil, err
	}
	return getEmployee(ctx, tx, id)
}
func lockEmployee(ctx context.Context, tx *sql.Tx, id string) error {
	var locked string
	return tx.QueryRowContext(ctx, `SELECT id FROM employees WHERE id=$1 FOR UPDATE`, id).Scan(&locked)
}
func patchEmployee(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	if len(in) == 0 {
		return nil, fail(422, "VALIDATION_FAILED", "Нужно хотя бы одно поле")
	}
	id := r.PathValue("employee_id")
	if err := lockEmployee(ctx, tx, id); err != nil {
		return nil, err
	}
	e, err := getEmployee(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if name, ok := in["display_name"]; ok {
		e.DisplayName = name.(string)
	}
	if active, ok := in["is_active"]; ok {
		if !active.(bool) {
			var has bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM batteries WHERE current_holder_employee_id=$1)`, id).Scan(&has); err != nil {
				return nil, err
			}
			if has {
				return nil, conflict("EMPLOYEE_HAS_CUSTODY")
			}
			if e.DisabledAt == nil {
				var now time.Time
				if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
					return nil, err
				}
				e.DisabledAt = &now
			}
		} else {
			e.DisabledAt = nil
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE employees SET display_name=$2,disabled_at=$3,updated_at=clock_timestamp() WHERE id=$1`, id, e.DisplayName, e.DisabledAt); err != nil {
		return nil, err
	}
	return getEmployee(ctx, tx, id)
}
func createCredential(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	id := r.PathValue("employee_id")
	if err := lockEmployee(ctx, tx, id); err != nil {
		return nil, err
	}
	// Serialize the historical-value policy, including races with card replacement.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,913407))`, in.str("value")); err != nil {
		return nil, err
	}
	var active, foreign bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM employee_credentials WHERE value=$1 AND disabled_at IS NULL),EXISTS(SELECT 1 FROM employee_credentials WHERE value=$1 AND employee_id<>$2)`, in.str("value"), id).Scan(&active, &foreign); err != nil {
		return nil, err
	}
	if active {
		return nil, conflict("ACTIVE_CREDENTIAL_EXISTS")
	}
	if foreign {
		return nil, conflict("CREDENTIAL_REUSE_UNCONFIRMED")
	}
	if old := in.str("replaces_credential_id"); old != "" {
		if _, err := getCredential(ctx, tx, id, old); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE employee_credentials SET disabled_at=COALESCE(disabled_at,clock_timestamp()),updated_at=CASE WHEN disabled_at IS NULL THEN clock_timestamp() ELSE updated_at END WHERE employee_id=$1 AND id=$2`, id, old); err != nil {
			return nil, err
		}
	}
	cid := newID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO employee_credentials(id,employee_id,value) VALUES($1,$2,$3)`, cid, id, in.str("value")); err != nil {
		return nil, err
	}
	return getCredential(ctx, tx, id, cid)
}
func disableCredential(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	if in["is_active"].(bool) {
		return nil, fail(422, "VALIDATION_FAILED", "Карту можно только отключить")
	}
	eid, cid := r.PathValue("employee_id"), r.PathValue("credential_id")
	if err := lockEmployee(ctx, tx, eid); err != nil {
		return nil, err
	}
	if _, err := getCredential(ctx, tx, eid, cid); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE employee_credentials SET disabled_at=COALESCE(disabled_at,clock_timestamp()),updated_at=CASE WHEN disabled_at IS NULL THEN clock_timestamp() ELSE updated_at END WHERE employee_id=$1 AND id=$2`, eid, cid); err != nil {
		return nil, err
	}
	return getCredential(ctx, tx, eid, cid)
}

// Lock order is employee then credential, shared with all administrative writes.
func commandActor(ctx context.Context, tx *sql.Tx, value string) (eid, cid string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT employee_id,id FROM employee_credentials WHERE value=$1 ORDER BY disabled_at NULLS FIRST,created_at DESC,id DESC LIMIT 1`, value).Scan(&eid, &cid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fail(404, "CREDENTIAL_NOT_FOUND", "Карта не найдена")
	}
	if err != nil {
		return
	}
	var disabled *time.Time
	if err = tx.QueryRowContext(ctx, `SELECT disabled_at FROM employees WHERE id=$1 FOR SHARE`, eid).Scan(&disabled); err != nil {
		return
	}
	if disabled != nil {
		return "", "", fail(403, "EMPLOYEE_INACTIVE", "Сотрудник отключён")
	}
	if err = tx.QueryRowContext(ctx, `SELECT disabled_at FROM employee_credentials WHERE id=$1 AND employee_id=$2 FOR SHARE`, cid, eid).Scan(&disabled); err != nil {
		return
	}
	if disabled != nil {
		return "", "", fail(403, "CREDENTIAL_INACTIVE", "Карта отключена")
	}
	return
}
func (s *Server) resolveCredential(w http.ResponseWriter, r *http.Request) {
	in, err := decodeInput(w, r, "credential_value", false)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	eid, cid, err := commandActor(ctx, tx, in.str("credential_value"))
	if err != nil {
		writeError(w, err)
		return
	}
	e, err := getEmployee(ctx, tx, eid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"employee": e, "credential_id": cid})
}
