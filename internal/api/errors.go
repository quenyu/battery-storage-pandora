package api

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"log/slog"
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
func invalid(message string) error                { return fail(400, "VALIDATION_ERROR", message) }
func conflict(code string) error {
	return fail(409, code, "Конфликт состояния или уникальности")
}
func notFound() error { return fail(404, "RESOURCE_NOT_FOUND", "Ресурс не найден") }

func writeError(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		var pgErr *pgconn.PgError
		var netErr net.Error
		switch {
		case errors.Is(err, sql.ErrNoRows):
			apiErr = &APIError{404, "RESOURCE_NOT_FOUND", "Ресурс не найден"}
		case errors.As(err, &pgErr):
			switch {
			case pgErr.Code == "23505":
				codes := map[string]string{"one_battery_per_cell": "CELL_OCCUPIED", "batteries_inventory_code_key": "INVENTORY_CODE_EXISTS", "employee_credentials_barcode_key": "BARCODE_EXISTS", "cabinets_number_key": "NUMBER_EXISTS", "shelves_cabinet_id_number_key": "NUMBER_EXISTS", "cells_shelf_id_number_key": "NUMBER_EXISTS"}
				if code, ok := codes[pgErr.ConstraintName]; ok {
					apiErr = &APIError{409, code, "Значение уже используется"}
				}
			case pgErr.Code == "23503":
				apiErr = &APIError{404, "RESOURCE_NOT_FOUND", "Связанный ресурс не найден"}
			case pgErr.Code == "40P01" || pgErr.Code == "55P03" || pgErr.Code == "57014" || pgErr.Code == "40001" || strings.HasPrefix(pgErr.Code, "08") || strings.HasPrefix(pgErr.Code, "53") || strings.HasPrefix(pgErr.Code, "57P"):
				apiErr = &APIError{503, "TEMPORARILY_UNAVAILABLE", "База данных временно недоступна; повторите команду с тем же ключом"}
			}
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.As(err, &netErr):
			apiErr = &APIError{503, "TEMPORARILY_UNAVAILABLE", "База данных временно недоступна"}
		}
	}
	if apiErr == nil {
		slog.Error("request failed", "error", err)
		apiErr = &APIError{500, "INTERNAL_ERROR", "Внутренняя ошибка"}
	}
	writeJSON(w, apiErr.Status, apiErr)
}
