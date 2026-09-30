package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type filters struct {
	limit, offset int
	values        url.Values
}

func parseFilters(r *http.Request, allowed string) (filters, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	f := filters{limit: 50, offset: 0, values: q}
	if err != nil {
		return f, invalid("Некорректная query-строка")
	}
	keys := map[string]bool{"limit": true, "offset": true}
	for _, k := range strings.Fields(allowed) {
		keys[k] = true
	}
	for key, v := range q {
		if !keys[key] || len(v) != 1 || v[0] == "" {
			return f, invalid("Неизвестный, пустой или повторный query-параметр")
		}
		s := v[0]
		switch key {
		case "limit", "offset":
			// Decimal unsigned notation only; int32 is part of the API contract.
			if strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return f, invalid("Неверная пагинация")
			}
			n, err := strconv.ParseInt(s, 10, 32)
			if err != nil {
				return f, invalid("Неверная пагинация")
			}
			if key == "limit" {
				if n < 1 || n > 100 {
					return f, invalid("limit должен быть 1–100")
				}
				f.limit = int(n)
			} else {
				f.offset = int(n)
			}
		case "status":
			if s != "stored" && s != "issued" && s != "lost" {
				return f, invalid("Неизвестный статус")
			}
		case "inventory_code":
			if !codePattern.MatchString(s) {
				return f, invalid("Неверный инвентарный код")
			}
		case "from", "to":
			if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
				return f, invalid("Неверная дата RFC3339")
			}
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
			return f, invalid("from должен быть раньше to")
		}
	}
	return f, nil
}

func listRows[T any](ctx context.Context, q queryer, query string, args []any, f filters, scan func(scanner) (T, error)) (Page, error) {
	args = append(args, f.limit, f.offset)
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
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
	return Page{Items: items, Limit: f.limit, Offset: f.offset}, rows.Err()
}
func (s *Server) listEmployees(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	return listRows(ctx, s.db, employeeSelect+` ORDER BY e.id`, nil, f, scanEmployee)
}
func (s *Server) listCabinets(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	return listRows(ctx, s.db, `SELECT id,number FROM cabinets ORDER BY id`, nil, f, scanCabinet)
}
func (s *Server) listShelves(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	id := r.PathValue("cabinet_id")
	var exists bool
	if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cabinets WHERE id=$1)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound()
	}
	return listRows(ctx, s.db, `SELECT id,cabinet_id,number FROM shelves WHERE cabinet_id=$1 ORDER BY id`, []any{id}, f, scanShelf)
}
func (s *Server) listCells(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	id := r.PathValue("shelf_id")
	var exists bool
	if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM shelves WHERE id=$1)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound()
	}
	return listRows(ctx, s.db, cellSelect+` WHERE c.shelf_id=$1 ORDER BY c.id`, []any{id}, f, scanCell)
}
func (s *Server) listBatteries(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "status holder_employee_id cell_id inventory_code")
	if err != nil {
		return nil, err
	}
	query := batterySelect + ` WHERE true`
	args := []any{}
	for _, key := range []string{"status", "holder_employee_id", "cell_id", "inventory_code"} {
		if f.values.Has(key) {
			args = append(args, f.values.Get(key))
			query += fmt.Sprintf(" AND b.%s=$%d", key, len(args))
		}
	}
	return listRows(ctx, s.db, query+` ORDER BY b.id`, args, f, scanBattery)
}
func (s *Server) listOperations(ctx context.Context, r *http.Request) (any, error) {
	f, err := parseFilters(r, "battery_id employee_id cell_id from to")
	if err != nil {
		return nil, err
	}
	query := operationSelect + ` WHERE true`
	args := []any{}
	// Column expressions are constants; all user values remain SQL parameters.
	for _, filter := range []struct{ key, expression string }{
		{"battery_id", "battery_id=$%[1]d"},
		{"employee_id", "(actor_employee_id=$%[1]d OR from_holder_employee_id=$%[1]d OR to_holder_employee_id=$%[1]d)"},
		{"cell_id", "(from_cell_id=$%[1]d OR to_cell_id=$%[1]d)"},
		{"from", "occurred_at >= $%[1]d"}, {"to", "occurred_at < $%[1]d"},
	} {
		if f.values.Has(filter.key) {
			var value any = f.values.Get(filter.key)
			if filter.key == "from" || filter.key == "to" {
				boundary, _ := time.Parse(time.RFC3339Nano, value.(string))
				// PostgreSQL stores microseconds. Ceil both range endpoints so a
				// sub-microsecond boundary preserves >= from and < to semantics.
				if remainder := boundary.Nanosecond() % 1000; remainder != 0 {
					boundary = boundary.Add(time.Duration(1000-remainder) * time.Nanosecond)
				}
				value = boundary
			}
			args = append(args, value)
			query += " AND " + fmt.Sprintf(filter.expression, len(args))
		}
	}
	return listRows(ctx, s.db, query+` ORDER BY occurred_at DESC,id DESC`, args, f, scanOperation)
}
