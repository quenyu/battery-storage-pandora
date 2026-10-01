package handler

import (
	"battery-storage-pandora/internal/model"
	"net/http"
	"net/url"
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
		case "status":
			if value != "STORED" && value != "ISSUED" {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Неизвестный статус")
			}
		case "type":
			if value != "STORE" && value != "TAKE" && value != "RETURN" && value != "MOVE" {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Неизвестный тип операции")
			}
		case "inventory_code":
			// Inventory codes are literal identifiers, including their spaces.
			if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || strings.ContainsRune(value, 0) {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Пустой код или запрещённый символ")
			}
		case "location", "device_code":
			if !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
				strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Недопустимый текстовый фильтр")
			}
			if name == "location" && !model.ValidLocation(value) {
				return nil, model.NewError(422, "VALIDATION_FAILED", "Неверный адрес")
			}
		case "from", "to":
			boundary, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return nil, model.Invalid("Неверная дата RFC3339")
			}
			value = boundary.UTC().Format(time.RFC3339Nano)
		default:
			if !model.ValidUUID(value) {
				return nil, model.Invalid("Неверный UUID фильтра")
			}
			value = strings.ToLower(value)
		}
		filters[name] = value
	}
	if filters["from"] != "" && filters["to"] != "" {
		from, _ := time.Parse(time.RFC3339Nano, filters["from"])
		to, _ := time.Parse(time.RFC3339Nano, filters["to"])
		if !from.Before(to) {
			return nil, model.NewError(422, "VALIDATION_FAILED", "from должен быть раньше to")
		}
	}
	return filters, nil
}
