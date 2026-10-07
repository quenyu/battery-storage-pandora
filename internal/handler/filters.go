package handler

import (
	"battery-storage-pandora/internal/model"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func parseFilters(r *http.Request, allowed string) (model.Filters, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, model.Invalid("Некорректная query-строка")
	}
	fields := make(map[string]bool)
	for _, field := range strings.Fields(allowed) {
		fields[field] = true
	}
	filters := model.Filters{}
	for name, values := range query {
		if !fields[name] || len(values) != 1 || values[0] == "" {
			return nil, model.Invalid("Неизвестный, пустой или повторный query-параметр")
		}
		value := values[0]
		switch name {
		case "is_active":
			if value != "true" && value != "false" {
				return nil, model.Invalid("is_active должен быть true или false")
			}
			filters[name] = value == "true"
		case "status":
			if value != model.BatteryStored && value != model.BatteryIssued {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Неизвестный статус")
			}
			filters[name] = value
		case "type":
			if value != model.OperationStore && value != model.OperationTake && value != model.OperationReturn && value != model.OperationMove {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Неизвестный тип операции")
			}
			filters[name] = value
		case "inventory_code":
			// Inventory codes are literal identifiers, including their spaces.
			if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || strings.ContainsRune(value, 0) {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Пустой код или запрещённый символ")
			}
			filters[name] = value
		case "location":
			if err := validateLocations(&value); err != nil {
				return nil, err
			}
			filters[name] = value
		case "from", "to":
			boundary, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return nil, model.Invalid("Неверная дата RFC3339")
			}
			filters[name] = boundary.UTC()
		default:
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil || id < 1 {
				return nil, model.Invalid("Неверный идентификатор в фильтре")
			}
			filters[name] = id
		}
	}
	from, hasFrom := filters["from"].(time.Time)
	to, hasTo := filters["to"].(time.Time)
	if hasFrom && hasTo && !from.Before(to) {
		return nil, model.NewError(422, "VALIDATION_FAILED", "from должен быть раньше to")
	}
	return filters, nil
}
