package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

func ClassifyError(err error) *model.Error {
	var apiError *model.Error
	if errors.As(err, &apiError) {
		return apiError
	}
	if errors.Is(err, sql.ErrNoRows) {
		return model.NotFound()
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		if databaseError.Code == "23505" {
			conflicts := map[string]string{
				"batteries_inventory_code_key":           "INVENTORY_CODE_EXISTS",
				"credentials_active_value_uq":            "ACTIVE_CREDENTIAL_EXISTS",
				"credentials_one_active_per_employee_uq": "EMPLOYEE_ACTIVE_CREDENTIAL_EXISTS",
				"employees_personnel_number_key":         "PERSONNEL_NUMBER_EXISTS",
			}
			if code, known := conflicts[databaseError.ConstraintName]; known {
				return model.Conflict(code)
			}
		}
		if temporaryDatabaseError(databaseError.Code) {
			return model.NewError(503, "TEMPORARILY_UNAVAILABLE", "Временный сбой; проверьте текущее состояние перед повтором")
		}
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &networkError) {
		return model.NewError(503, "TEMPORARILY_UNAVAILABLE", "Временный сбой; проверьте текущее состояние перед повтором")
	}
	return model.NewError(500, "INTERNAL_ERROR", "Внутренняя ошибка")
}

func temporaryDatabaseError(code string) bool {
	switch code {
	case "40P01", "55P03", "57014", "40001":
		return true // Deadlock, lock timeout, statement timeout or serialization failure.
	}
	return strings.HasPrefix(code, "08") || strings.HasPrefix(code, "53") || strings.HasPrefix(code, "57P")
}
