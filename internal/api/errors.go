package api

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"net"
	"net/http"
	"strings"
)

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string                 { return e.Code + ": " + e.Message }
func fail(status int, code, message string) error { return &APIError{status, code, message} }
func invalid(message string) error                { return fail(400, "INVALID_REQUEST", message) }
func conflict(code string) error {
	return fail(409, code, "Конфликт состояния или уникальности")
}
func notFound() error { return fail(404, "RESOURCE_NOT_FOUND", "Ресурс не найден") }
func classify(err error) *APIError {
	var a *APIError
	if errors.As(err, &a) {
		return a
	}
	if errors.Is(err, sql.ErrNoRows) {
		return notFound().(*APIError)
	}
	var pg *pgconn.PgError
	var ne net.Error
	if errors.As(err, &pg) {
		if pg.Code == "23505" {
			codes := map[string]string{"batteries_one_per_location_uq": "LOCATION_OCCUPIED", "batteries_inventory_code_key": "INVENTORY_CODE_EXISTS", "credentials_active_value_uq": "ACTIVE_CREDENTIAL_EXISTS", "employees_personnel_number_key": "PERSONNEL_NUMBER_EXISTS"}
			if code, ok := codes[pg.ConstraintName]; ok {
				return conflict(code).(*APIError)
			}
		}
		if pg.Code == "40P01" || pg.Code == "55P03" || pg.Code == "57014" || pg.Code == "40001" || strings.HasPrefix(pg.Code, "08") || strings.HasPrefix(pg.Code, "53") || strings.HasPrefix(pg.Code, "57P") {
			return fail(503, "TEMPORARILY_UNAVAILABLE", "Повторите запрос с тем же ключом").(*APIError)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &ne) {
		return fail(503, "TEMPORARILY_UNAVAILABLE", "Повторите запрос с тем же ключом").(*APIError)
	}
	return fail(500, "INTERNAL_ERROR", "Внутренняя ошибка").(*APIError)
}
func errorBody(a *APIError, requestID string) any {
	return map[string]any{"error": map[string]any{"code": a.Code, "message": a.Message, "details": map[string]any{}, "request_id": requestID}}
}
func writeError(w http.ResponseWriter, err error) {
	a := classify(err)
	writeJSON(w, a.Status, errorBody(a, w.Header().Get("X-Request-ID")))
}
