package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var addressFilterPattern = regexp.MustCompile(`^[1-9][0-9]*\.[1-9][0-9]*\.[1-9][0-9]*$`)

type filters struct {
	limit  int
	values url.Values
	scope  string
	cursor *pageCursor
}

// A cursor is a continuation hint, never an authorization credential. The
// checksum detects corruption; scope binds it to its endpoint and filters.
type pageCursor struct {
	Scope        string `json:"scope"`
	Mode         string `json:"mode"`
	ID           string `json:"id"`
	Time         string `json:"time,omitempty"`
	Version      int64  `json:"version,omitempty"`
	UpperID      string `json:"upper_id,omitempty"`
	UpperTime    string `json:"upper_time,omitempty"`
	UpperVersion int64  `json:"upper_version,omitempty"`
}

func parseFilters(r *http.Request, allowed string) (filters, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	f := filters{limit: 50, values: q}
	if err != nil {
		return f, invalid("Некорректная query-строка")
	}
	keys := map[string]bool{"limit": true, "cursor": true}
	for _, k := range strings.Fields(allowed) {
		keys[k] = true
	}
	for key, v := range q {
		if !keys[key] || len(v) != 1 || v[0] == "" {
			return f, invalid("Неизвестный, пустой или повторный query-параметр")
		}
		s := v[0]
		switch key {
		case "limit":
			if strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return f, invalid("Неверный limit")
			}
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 200 {
				return f, invalid("limit должен быть 1–200")
			}
			f.limit = n
		case "cursor":
		case "status":
			if s != "STORED" && s != "ISSUED" {
				return f, fail(422, "VALIDATION_FAILED", "Неизвестный статус")
			}
		case "type":
			if s != "STORE" && s != "TAKE" && s != "RETURN" && s != "MOVE" {
				return f, fail(422, "VALIDATION_FAILED", "Неизвестный тип операции")
			}
		case "is_active":
			if s != "true" && s != "false" {
				return f, invalid("is_active должен быть true или false")
			}
		case "inventory_code":
			// Match registration's literal identifier policy, including spaces.
			if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || strings.ContainsRune(s, 0) {
				return f, fail(422, "VALIDATION_FAILED", "Пустой код или запрещённый символ")
			}
		case "location", "device_code":
			if !utf8.ValidString(s) || strings.TrimSpace(s) != s || strings.IndexFunc(s, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
				return f, fail(422, "VALIDATION_FAILED", "Текстовый фильтр не должен содержать крайние пробелы или управляющие символы")
			}
			if key == "location" && !addressFilterPattern.MatchString(s) {
				return f, fail(422, "VALIDATION_FAILED", "Адрес должен иметь вид шкаф.полка.ячейка: положительные числа без ведущих нулей")
			}
		case "from", "to":
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				return f, invalid("Неверная дата RFC3339")
			}
			q.Set(key, t.UTC().Format(time.RFC3339Nano))
		default:
			if !uuidPattern.MatchString(s) {
				return f, invalid("Неверный UUID фильтра")
			}
			q.Set(key, strings.ToLower(s))
		}
	}
	if q.Has("from") && q.Has("to") {
		a, _ := time.Parse(time.RFC3339Nano, q.Get("from"))
		b, _ := time.Parse(time.RFC3339Nano, q.Get("to"))
		if !a.Before(b) {
			return f, fail(422, "VALIDATION_FAILED", "from должен быть раньше to")
		}
	}
	bound := url.Values{}
	for k, v := range q {
		if k != "limit" && k != "cursor" {
			bound[k] = v
		}
	}
	f.scope = fmt.Sprintf("%x", sha256.Sum256([]byte(canonicalPath(r)+"?"+bound.Encode())))
	if q.Has("cursor") {
		c, err := decodeCursor(q.Get("cursor"))
		if err != nil || c.Scope != f.scope {
			return f, invalid("Неверный курсор или несовпадение endpoint/фильтров")
		}
		f.cursor = &c
	}
	return f, nil
}

func encodeCursor(c pageCursor) string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(sum[:])
}

func decodeCursor(token string) (pageCursor, error) {
	var c pageCursor
	if len(token) > 4096 {
		return c, fmt.Errorf("cursor too large")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return c, fmt.Errorf("cursor envelope")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, err
	}
	sum := sha256.Sum256(raw)
	if parts[1] != hex.EncodeToString(sum[:]) {
		return c, fmt.Errorf("cursor checksum")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("cursor trailing data")
	}
	if !uuidPattern.MatchString(c.ID) {
		return c, fmt.Errorf("cursor id")
	}
	switch c.Mode {
	case "id":
		if c.Time != "" || c.Version != 0 || c.UpperID != "" || c.UpperTime != "" || c.UpperVersion != 0 {
			return c, fmt.Errorf("cursor fields")
		}
	case "version":
		if c.Version < 1 || c.UpperVersion < c.Version || c.Time != "" || c.UpperTime != "" || c.UpperID != "" {
			return c, fmt.Errorf("cursor version")
		}
	case "time":
		t, err := time.Parse(time.RFC3339Nano, c.Time)
		if err != nil {
			return c, err
		}
		u, err := time.Parse(time.RFC3339Nano, c.UpperTime)
		if err != nil {
			return c, err
		}
		if !uuidPattern.MatchString(c.UpperID) || u.Before(t) || (u.Equal(t) && c.UpperID < c.ID) || c.Version != 0 || c.UpperVersion != 0 {
			return c, fmt.Errorf("cursor boundary")
		}
	default:
		return c, fmt.Errorf("cursor mode")
	}
	return c, nil
}

func listRows[T any](ctx context.Context, q queryer, query string, args []any, f filters, alias, mode string, scan func(scanner) (T, error)) (Page, error) {
	c := pageCursor{Scope: f.scope, Mode: mode}
	if f.cursor != nil {
		c = *f.cursor
		if c.Mode != mode {
			return Page{}, invalid("Неверный порядок курсора")
		}
		switch mode {
		case "id":
			args = append(args, c.ID)
			query += fmt.Sprintf(" AND %s.id > $%d", alias, len(args))
		case "version":
			args = append(args, c.Version, c.UpperVersion)
			query += fmt.Sprintf(" AND o.battery_version < $%d AND o.battery_version <= $%d", len(args)-1, len(args))
		case "time":
			t, _ := time.Parse(time.RFC3339Nano, c.Time)
			u, _ := time.Parse(time.RFC3339Nano, c.UpperTime)
			args = append(args, t, c.ID, u, c.UpperID)
			query += fmt.Sprintf(" AND (o.occurred_at,o.id) < ($%d,$%d) AND (o.occurred_at,o.id) <= ($%d,$%d)", len(args)-3, len(args)-2, len(args)-1, len(args))
		}
	}
	switch mode {
	case "id":
		query += " ORDER BY " + alias + ".id"
	case "version":
		query += " ORDER BY o.battery_version DESC"
	case "time":
		query += " ORDER BY o.occurred_at DESC,o.id DESC"
	}
	args = append(args, f.limit+1)
	query += fmt.Sprintf(" LIMIT $%d", len(args))
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := make([]T, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return Page{}, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(items) > f.limit {
		items = items[:f.limit]
		page.Items = items
		if f.cursor == nil {
			if o, ok := any(items[0]).(Operation); ok {
				if mode == "version" {
					c.UpperVersion = o.BatteryVersion
				} else {
					c.UpperTime = o.OccurredAt.Format(time.RFC3339Nano)
					c.UpperID = o.ID
				}
			}
		}
		switch last := any(items[len(items)-1]).(type) {
		case Employee:
			c.ID = last.ID
		case Credential:
			c.ID = last.ID
		case Battery:
			c.ID = last.ID
		case Operation:
			c.ID = last.ID
			if mode == "version" {
				c.Version = last.BatteryVersion
			} else {
				c.Time = last.OccurredAt.Format(time.RFC3339Nano)
			}
		}
		next := encodeCursor(c)
		page.NextCursor = &next
	}
	return page, nil
}

func (s *Server) listEmployees(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "is_active")
	if err != nil {
		return nil, err
	}
	query := employeeSelect + ` WHERE true`
	if f.values.Has("is_active") {
		if f.values.Get("is_active") == "true" {
			query += ` AND e.disabled_at IS NULL`
		} else {
			query += ` AND e.disabled_at IS NOT NULL`
		}
	}
	return listRows(ctx, s.db, query, nil, f, "e", "id", scanEmployee)
}

func (s *Server) listCredentials(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	id := r.PathValue("employee_id")
	if _, err = getEmployee(ctx, s.db, id); err != nil {
		return nil, err
	}
	return listRows(ctx, s.db, credentialSelect+` WHERE c.employee_id=$1`, []any{id}, f, "c", "id", scanCredential)
}

func (s *Server) listBatteries(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "inventory_code status holder_employee_id location")
	if err != nil {
		return nil, err
	}
	query, args := batteryFilters(f)
	return listRows(ctx, s.db, query, args, f, "b", "id", scanBattery)
}

func batteryFilters(f filters) (string, []any) {
	query := batterySelect + ` WHERE true`
	args := []any{}
	for _, p := range []struct{ key, column string }{{"inventory_code", "inventory_code"}, {"status", "status"}, {"holder_employee_id", "current_holder_employee_id"}, {"location", "current_location"}} {
		if f.values.Has(p.key) {
			args = append(args, f.values.Get(p.key))
			query += fmt.Sprintf(" AND b.%s=$%d", p.column, len(args))
		}
	}
	return query, args
}

func (s *Server) listCustody(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	id := r.PathValue("employee_id")
	if _, err = getEmployee(ctx, s.db, id); err != nil {
		return nil, err
	}
	return listRows(ctx, s.db, batterySelect+` WHERE b.current_holder_employee_id=$1`, []any{id}, f, "b", "id", scanBattery)
}

func (s *Server) listBatteryOperations(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	id := r.PathValue("battery_id")
	if _, err = getBattery(ctx, s.db, id); err != nil {
		return nil, err
	}
	return listRows(ctx, s.db, operationSelect+` WHERE o.battery_id=$1`, []any{id}, f, "o", "version", scanOperation)
}

func (s *Server) listEmployeeOperations(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	id := r.PathValue("employee_id")
	if _, err = getEmployee(ctx, s.db, id); err != nil {
		return nil, err
	}
	return listRows(ctx, s.db, operationSelect+` WHERE (o.actor_employee_id=$1 OR o.source_holder_employee_id=$1 OR o.destination_holder_employee_id=$1)`, []any{id}, f, "o", "time", scanOperation)
}

func (s *Server) listOperations(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "battery_id employee_id location device_code type from to")
	if err != nil {
		return nil, err
	}
	query := operationSelect + ` WHERE true`
	args := []any{}
	for _, p := range []struct{ key, expression string }{
		{"battery_id", "o.battery_id=$%[1]d"},
		{"employee_id", "(o.actor_employee_id=$%[1]d OR o.source_holder_employee_id=$%[1]d OR o.destination_holder_employee_id=$%[1]d)"},
		{"location", "(o.source_location=$%[1]d OR o.destination_location=$%[1]d)"},
		{"device_code", "o.device_code=$%[1]d"}, {"type", "o.type=$%[1]d"},
		{"from", "o.occurred_at >= $%[1]d"}, {"to", "o.occurred_at < $%[1]d"},
	} {
		if !f.values.Has(p.key) {
			continue
		}
		var value any = f.values.Get(p.key)
		if p.key == "from" || p.key == "to" {
			t, _ := time.Parse(time.RFC3339Nano, value.(string))
			// Ceil to PostgreSQL microseconds to preserve inclusive/exclusive
			// semantics when the supplied boundary contains nanoseconds.
			if n := t.Nanosecond() % 1000; n != 0 {
				t = t.Add(time.Duration(1000-n) * time.Nanosecond)
			}
			value = t
		}
		args = append(args, value)
		query += " AND " + fmt.Sprintf(p.expression, len(args))
	}
	return listRows(ctx, s.db, query, args, f, "o", "time", scanOperation)
}
