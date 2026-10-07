package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"fmt"
	"time"
)

func (r *Repository) ListUsers(ctx context.Context, filters model.Filters) ([]model.User, error) {
	query := userSelect + ` WHERE true`
	if active, supplied := filters["is_active"]; supplied {
		if active == true {
			query += ` AND u.disabled_at IS NULL`
		} else {
			query += ` AND u.disabled_at IS NOT NULL`
		}
	}
	query += ` ORDER BY u.id`
	return queryList(ctx, r, query, scanUser)
}

func (r *Repository) ListBatteries(ctx context.Context, filters model.Filters) ([]model.Battery, error) {
	query := batterySelect + ` WHERE true`
	args := []any{}
	for _, filter := range []struct{ key, expression string }{
		{"inventory_code", "b.inventory_code = $%d"},
		{"holder_user_id", "l.type = 'TAKE' AND l.user_id = $%d"},
		{"location", "l.location = $%d"},
	} {
		if value, supplied := filters[filter.key]; supplied {
			args = append(args, value)
			query += " AND " + fmt.Sprintf(filter.expression, len(args))
		}
	}
	switch filters["status"] {
	case model.BatteryIssued:
		query += ` AND l.type = 'TAKE'`
	case model.BatteryStored:
		query += ` AND l.type <> 'TAKE'`
	}
	query += ` ORDER BY b.id`
	return queryList(ctx, r, query, scanBattery, args...)
}

func (r *Repository) ListOperations(ctx context.Context, filters model.Filters) ([]model.Operation, error) {
	query := operationSelect + ` WHERE true`
	args := []any{}
	for _, filter := range []struct{ key, expression string }{
		{"battery_id", "o.battery_id = $%[1]d"},
		{"user_id", "o.actor_user_id = $%[1]d"},
		{"location", "(o.source_location = $%[1]d OR o.destination_location = $%[1]d)"},
		{"type", "o.type = $%[1]d"},
		{"from", "o.occurred_at >= $%[1]d"}, {"to", "o.occurred_at < $%[1]d"},
	} {
		value, supplied := filters[filter.key]
		if !supplied {
			continue
		}
		if boundary, ok := value.(time.Time); ok {
			value = postgresTimeBoundary(boundary)
		}
		args = append(args, value)
		query += " AND " + fmt.Sprintf(filter.expression, len(args))
	}
	query += ` ORDER BY o.id DESC`
	return queryList(ctx, r, query, scanOperation, args...)
}

// PostgreSQL timestamps have microsecond precision; rounding up preserves both
// the inclusive from and exclusive to bounds.
func postgresTimeBoundary(boundary time.Time) time.Time {
	if remainder := boundary.Nanosecond() % 1000; remainder != 0 {
		boundary = boundary.Add(time.Duration(1000-remainder) * time.Nanosecond)
	}
	return boundary
}

func queryList[T any](ctx context.Context, r *Repository, query string, scan func(scanner) (T, error), args ...any) ([]T, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
