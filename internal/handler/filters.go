package handler

import (
	"battery-storage-pandora/internal/model"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func parseFilters(r *http.Request, allowed string) (model.ListOptions, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	f := model.ListOptions{Limit: 50, Values: map[string]string{}}
	if err != nil {
		return f, model.Invalid("Некорректная query-строка")
	}
	keys := map[string]bool{"limit": true, "cursor": true}
	for _, k := range strings.Fields(allowed) {
		keys[k] = true
	}
	for key, v := range q {
		if !keys[key] || len(v) != 1 || v[0] == "" {
			return f, model.Invalid("Неизвестный, пустой или повторный query-параметр")
		}
		s := v[0]
		switch key {
		case "limit":
			if strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return f, model.Invalid("Неверный limit")
			}
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 200 {
				return f, model.Invalid("limit должен быть 1–200")
			}
			f.Limit = n
		case "cursor":
		case "status":
			if s != "STORED" && s != "ISSUED" {
				return f, model.NewError(422, "VALIDATION_FAILED", "Неизвестный статус")
			}
		case "type":
			if s != "STORE" && s != "TAKE" && s != "RETURN" && s != "MOVE" {
				return f, model.NewError(422, "VALIDATION_FAILED", "Неизвестный тип операции")
			}
		case "is_active":
			if s != "true" && s != "false" {
				return f, model.Invalid("is_active должен быть true или false")
			}
		case "inventory_code":
			// Match registration's literal identifier policy, including spaces.
			if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || strings.ContainsRune(s, 0) {
				return f, model.NewError(422, "VALIDATION_FAILED", "Пустой код или запрещённый символ")
			}
		case "location", "device_code":
			if !utf8.ValidString(s) || strings.TrimSpace(s) != s || strings.IndexFunc(s, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
				return f, model.NewError(422, "VALIDATION_FAILED", "Текстовый фильтр не должен содержать крайние пробелы или управляющие символы")
			}
			if key == "location" && !model.ValidLocation(s) {
				return f, model.NewError(422, "VALIDATION_FAILED", "Адрес должен иметь вид шкаф.полка.ячейка: положительные числа без ведущих нулей")
			}
		case "from", "to":
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				return f, model.Invalid("Неверная дата RFC3339")
			}
			q.Set(key, t.UTC().Format(time.RFC3339Nano))
		default:
			if !model.ValidUUID(s) {
				return f, model.Invalid("Неверный UUID фильтра")
			}
			q.Set(key, strings.ToLower(s))
		}
	}
	if q.Has("from") && q.Has("to") {
		a, _ := time.Parse(time.RFC3339Nano, q.Get("from"))
		b, _ := time.Parse(time.RFC3339Nano, q.Get("to"))
		if !a.Before(b) {
			return f, model.NewError(422, "VALIDATION_FAILED", "from должен быть раньше to")
		}
	}
	bound := url.Values{}
	for k, v := range q {
		if k != "limit" && k != "cursor" {
			bound[k] = v
		}
	}
	f.Scope = fmt.Sprintf("%x", sha256.Sum256([]byte(canonicalPath(r)+"?"+bound.Encode())))
	if q.Has("cursor") {
		c, err := model.DecodeCursor(q.Get("cursor"))
		if err != nil || c.Scope != f.Scope {
			return f, model.Invalid("Неверный курсор или несовпадение endpoint/фильтров")
		}
		f.Cursor = &c
	}
	for key, values := range q {
		f.Values[key] = values[0]
	}
	return f, nil
}
