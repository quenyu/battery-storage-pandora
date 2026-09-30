package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

func createEmployee(ctx context.Context, tx *sql.Tx, request *http.Request, values input) (any, error) {
	employeeID := newID()
	_, err := tx.ExecContext(ctx, `
        INSERT INTO employees (id, display_name, personnel_number)
        VALUES ($1, $2, $3)
    `, employeeID, values.str("display_name"), values["personnel_number"])
	if err != nil {
		return nil, err
	}
	return getEmployee(ctx, tx, employeeID)
}

func lockEmployee(ctx context.Context, tx *sql.Tx, employeeID string) error {
	var lockedID string
	return tx.QueryRowContext(ctx, `SELECT id FROM employees WHERE id = $1 FOR UPDATE`, employeeID).Scan(&lockedID)
}

func patchEmployee(ctx context.Context, tx *sql.Tx, request *http.Request, values input) (any, error) {
	if len(values) == 0 {
		return nil, fail(422, "VALIDATION_FAILED", "Нужно хотя бы одно поле")
	}
	employeeID := request.PathValue("employee_id")
	if err := lockEmployee(ctx, tx, employeeID); err != nil {
		return nil, err
	}
	employee, err := getEmployee(ctx, tx, employeeID)
	if err != nil {
		return nil, err
	}
	if name, supplied := values["display_name"]; supplied {
		employee.DisplayName = name.(string)
	}
	if active, supplied := values["is_active"]; supplied {
		if active.(bool) {
			employee.DisabledAt = nil
		} else {
			var hasBatteries bool
			err := tx.QueryRowContext(ctx, `
                SELECT EXISTS (SELECT 1 FROM batteries WHERE current_holder_employee_id = $1)
            `, employeeID).Scan(&hasBatteries)
			if err != nil {
				return nil, err
			}
			if hasBatteries {
				return nil, conflict("EMPLOYEE_HAS_CUSTODY")
			}
			if employee.DisabledAt == nil {
				var disabledAt time.Time
				if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&disabledAt); err != nil {
					return nil, err
				}
				employee.DisabledAt = &disabledAt
			}
		}
	}
	_, err = tx.ExecContext(ctx, `
        UPDATE employees SET display_name = $2, disabled_at = $3, updated_at = clock_timestamp()
        WHERE id = $1
    `, employeeID, employee.DisplayName, employee.DisabledAt)
	if err != nil {
		return nil, err
	}
	return getEmployee(ctx, tx, employeeID)
}

func createCredential(ctx context.Context, tx *sql.Tx, request *http.Request, values input) (any, error) {
	employeeID := request.PathValue("employee_id")
	if err := lockEmployee(ctx, tx, employeeID); err != nil {
		return nil, err
	}
	value := values.str("value")
	// Serialize assignment of the same card value, including historical cards.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 913407))`, value); err != nil {
		return nil, err
	}
	var alreadyActive, usedByAnotherEmployee bool
	err := tx.QueryRowContext(ctx, `
        SELECT
            EXISTS (SELECT 1 FROM employee_credentials WHERE value = $1 AND disabled_at IS NULL),
            EXISTS (SELECT 1 FROM employee_credentials WHERE value = $1 AND employee_id <> $2)
    `, value, employeeID).Scan(&alreadyActive, &usedByAnotherEmployee)
	if err != nil {
		return nil, err
	}
	if alreadyActive {
		return nil, conflict("ACTIVE_CREDENTIAL_EXISTS")
	}
	if usedByAnotherEmployee {
		return nil, conflict("CREDENTIAL_REUSE_UNCONFIRMED")
	}
	if oldCredentialID := values.str("replaces_credential_id"); oldCredentialID != "" {
		if err := revokeCredential(ctx, tx, employeeID, oldCredentialID); err != nil {
			return nil, err
		}
	}
	credentialID := newID()
	_, err = tx.ExecContext(ctx, `
        INSERT INTO employee_credentials (id, employee_id, value) VALUES ($1, $2, $3)
    `, credentialID, employeeID, value)
	if err != nil {
		return nil, err
	}
	return getCredential(ctx, tx, employeeID, credentialID)
}

func disableCredential(ctx context.Context, tx *sql.Tx, request *http.Request, values input) (any, error) {
	if values["is_active"].(bool) {
		return nil, fail(422, "VALIDATION_FAILED", "Карту можно только отключить")
	}
	employeeID := request.PathValue("employee_id")
	credentialID := request.PathValue("credential_id")
	if err := lockEmployee(ctx, tx, employeeID); err != nil {
		return nil, err
	}
	if err := revokeCredential(ctx, tx, employeeID, credentialID); err != nil {
		return nil, err
	}
	return getCredential(ctx, tx, employeeID, credentialID)
}

func revokeCredential(ctx context.Context, tx *sql.Tx, employeeID, credentialID string) error {
	if _, err := getCredential(ctx, tx, employeeID, credentialID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
        UPDATE employee_credentials
        SET disabled_at = clock_timestamp(), updated_at = clock_timestamp()
        WHERE employee_id = $1 AND id = $2 AND disabled_at IS NULL
    `, employeeID, credentialID)
	return err
}

// Both commands and card changes lock employee first, then credential.
func commandActor(ctx context.Context, tx *sql.Tx, value string) (string, string, error) {
	var employeeID, credentialID string
	err := tx.QueryRowContext(ctx, `
        SELECT employee_id, id FROM employee_credentials WHERE value = $1
        ORDER BY disabled_at NULLS FIRST, created_at DESC, id DESC LIMIT 1
    `, value).Scan(&employeeID, &credentialID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fail(404, "CREDENTIAL_NOT_FOUND", "Карта не найдена")
	}
	if err != nil {
		return "", "", err
	}
	var disabledAt *time.Time
	err = tx.QueryRowContext(ctx, `SELECT disabled_at FROM employees WHERE id = $1 FOR SHARE`, employeeID).Scan(&disabledAt)
	if err != nil {
		return "", "", err
	}
	if disabledAt != nil {
		return "", "", fail(403, "EMPLOYEE_INACTIVE", "Сотрудник отключён")
	}
	err = tx.QueryRowContext(ctx, `
        SELECT disabled_at FROM employee_credentials WHERE id = $1 AND employee_id = $2 FOR SHARE
    `, credentialID, employeeID).Scan(&disabledAt)
	if err != nil {
		return "", "", err
	}
	if disabledAt != nil {
		return "", "", fail(403, "CREDENTIAL_INACTIVE", "Карта отключена")
	}
	return employeeID, credentialID, nil
}

func (s *Server) resolveCredential(w http.ResponseWriter, request *http.Request) {
	values, err := decodeInput(w, request, "credential_value", false)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	employeeID, credentialID, err := commandActor(ctx, tx, values.str("credential_value"))
	if err != nil {
		writeError(w, err)
		return
	}
	employee, err := getEmployee(ctx, tx, employeeID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"employee": employee, "credential_id": credentialID})
}
