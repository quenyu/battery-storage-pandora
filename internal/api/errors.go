package api

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (err *APIError) Error() string { return err.Code + ": " + err.Message }

func fail(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

func invalid(message string) *APIError { return fail(400, "INVALID_REQUEST", message) }
func conflict(code string) *APIError {
	return fail(409, code, "Конфликт состояния или уникальности")
}
func notFound() *APIError { return fail(404, "RESOURCE_NOT_FOUND", "Ресурс не найден") }

func classify(err error) *APIError {
	var apiError *APIError
	if errors.As(err, &apiError) {
		return apiError
	}
	if errors.Is(err, sql.ErrNoRows) {
		return notFound()
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		if databaseError.Code == "23505" {
			conflicts := map[string]string{
				"batteries_one_per_location_uq":  "LOCATION_OCCUPIED",
				"batteries_inventory_code_key":   "INVENTORY_CODE_EXISTS",
				"credentials_active_value_uq":    "ACTIVE_CREDENTIAL_EXISTS",
				"employees_personnel_number_key": "PERSONNEL_NUMBER_EXISTS",
			}
			if code, known := conflicts[databaseError.ConstraintName]; known {
				return conflict(code)
			}
		}
		if temporaryDatabaseError(databaseError.Code) {
			return fail(503, "TEMPORARILY_UNAVAILABLE", "Повторите запрос с тем же ключом")
		}
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &networkError) {
		return fail(503, "TEMPORARILY_UNAVAILABLE", "Повторите запрос с тем же ключом")
	}
	return fail(500, "INTERNAL_ERROR", "Внутренняя ошибка")
}

func temporaryDatabaseError(code string) bool {
	switch code {
	case "40P01", "55P03", "57014", "40001":
		return true // Deadlock, lock timeout, statement timeout or serialization failure.
	}
	return strings.HasPrefix(code, "08") || strings.HasPrefix(code, "53") || strings.HasPrefix(code, "57P")
}

type errorResponse struct {
	Error errorDetails `json:"error"`
}

type errorDetails struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	RequestID string         `json:"request_id"`
}

func errorBody(err *APIError, requestID string) errorResponse {
	return errorResponse{Error: errorDetails{
		Code:      err.Code,
		Message:   err.Message,
		Details:   map[string]any{},
		RequestID: requestID,
	}}
}

func writeError(w http.ResponseWriter, err error) {
	apiError := classify(err)
	writeJSON(w, apiError.Status, errorBody(apiError, w.Header().Get("X-Request-ID")))
}
