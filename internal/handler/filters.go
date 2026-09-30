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
	q, err := url.ParseQuery(r.URL.RawQuery)
	filters := model.Filters{}
	if err != nil {
		return filters, model.Invalid("Некорректная query-строка")
	}
	keys := map[string]bool{}
	for _, k := range strings.Fields(allowed) {
		keys[k] = true
	}
	for key, v := range q {
		if !keys[key] || len(v) != 1 || v[0] == "" {
			return filters, model.Invalid("Неизвестный, пустой или повторный query-параметр")
		}
		s := v[0]
		switch key {
		case "status":
			if s != "STORED" && s != "ISSUED" {
				return filters, model.NewError(422, "VALIDATION_FAILED", "Неизвестный статус")
			}
		case "type":
			if s != "STORE" && s != "TAKE" && s != "RETURN" && s != "MOVE" {
				return filters, model.NewError(422, "VALIDATION_FAILED", "Неизвестный тип операции")
			}
		case "is_active":
			if s != "true" && s != "false" {
				return filters, model.Invalid("is_active должен быть true или false")
			}
		case "inventory_code":
			// Match registration's literal identifier policy, including spaces.
			if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || strings.ContainsRune(s, 0) {
				return filters, model.NewError(422, "VALIDATION_FAILED", "Пустой код или запрещённый символ")
			}
		case "location", "device_code":
			if !utf8.ValidString(s) || strings.TrimSpace(s) != s || strings.IndexFunc(s, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
				return filters, model.NewError(422, "VALIDATION_FAILED", "Текстовый фильтр не должен содержать крайние пробелы или управляющие символы")
			}
			if key == "location" && !model.ValidLocation(s) {
				return filters, model.NewError(422, "VALIDATION_FAILED", "Адрес должен иметь вид шкаф.полка.ячейка: положительные числа без ведущих нулей")
			}
		case "from", "to":
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				return filters, model.Invalid("Неверная дата RFC3339")
			}
			q.Set(key, t.UTC().Format(time.RFC3339Nano))
		default:
			if !model.ValidUUID(s) {
				return filters, model.Invalid("Неверный UUID фильтра")
			}
			q.Set(key, strings.ToLower(s))
		}
	}
	if q.Has("from") && q.Has("to") {
		a, _ := time.Parse(time.RFC3339Nano, q.Get("from"))
		b, _ := time.Parse(time.RFC3339Nano, q.Get("to"))
		if !a.Before(b) {
			return filters, model.NewError(422, "VALIDATION_FAILED", "from должен быть раньше to")
		}
	}
	for key, values := range q {
		filters[key] = values[0]
	}
	return filters, nil
}
