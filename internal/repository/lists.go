package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"fmt"
	"time"
)

func (r *Repository) ListEmployees(ctx context.Context, options model.ListOptions) (model.Page, error) {
	query := employeeSelect + ` WHERE true`
	if active, supplied := options.Values["is_active"]; supplied {
		if active == "true" {
			query += ` AND e.disabled_at IS NULL`
		} else {
			query += ` AND e.disabled_at IS NOT NULL`
		}
	}
	return listRows(ctx, r.db, query, nil, options, "e", "id", scanEmployee)
}

func (r *Repository) ListCredentials(ctx context.Context, employeeID string, options model.ListOptions) (model.Page, error) {
	return listRows(ctx, r.db, credentialSelect+` WHERE c.employee_id = $1`,
		[]any{employeeID}, options, "c", "id", scanCredential)
}

func (r *Repository) ListBatteries(ctx context.Context, options model.ListOptions) (model.Page, error) {
	query := batterySelect + ` WHERE true`
	args := []any{}
	for _, filter := range []struct{ key, column string }{
		{"inventory_code", "inventory_code"},
		{"status", "status"},
		{"holder_employee_id", "current_holder_employee_id"},
		{"location", "current_location"},
	} {
		if value, supplied := options.Values[filter.key]; supplied {
			args = append(args, value)
			query += fmt.Sprintf(" AND b.%s = $%d", filter.column, len(args))
		}
	}
	return listRows(ctx, r.db, query, args, options, "b", "id", scanBattery)
}

func (r *Repository) ListCustody(ctx context.Context, employeeID string, options model.ListOptions) (model.Page, error) {
	return listRows(ctx, r.db, batterySelect+` WHERE b.current_holder_employee_id = $1`,
		[]any{employeeID}, options, "b", "id", scanBattery)
}

func (r *Repository) ListBatteryOperations(ctx context.Context, batteryID string, options model.ListOptions) (model.Page, error) {
	return listRows(ctx, r.db, operationSelect+` WHERE o.battery_id = $1`,
		[]any{batteryID}, options, "o", "version", scanOperation)
}

func (r *Repository) ListEmployeeOperations(ctx context.Context, employeeID string, options model.ListOptions) (model.Page, error) {
	return listRows(ctx, r.db, operationSelect+`
        WHERE (o.actor_employee_id = $1 OR o.source_holder_employee_id = $1 OR o.destination_holder_employee_id = $1)
    `, []any{employeeID}, options, "o", "time", scanOperation)
}

func (r *Repository) ListOperations(ctx context.Context, options model.ListOptions) (model.Page, error) {
	query := operationSelect + ` WHERE true`
	args := []any{}
	for _, filter := range []struct{ key, expression string }{
		{"battery_id", "o.battery_id=$%[1]d"},
		{"employee_id", "(o.actor_employee_id=$%[1]d OR o.source_holder_employee_id=$%[1]d OR o.destination_holder_employee_id=$%[1]d)"},
		{"location", "(o.source_location=$%[1]d OR o.destination_location=$%[1]d)"},
		{"device_code", "o.device_code=$%[1]d"}, {"type", "o.type=$%[1]d"},
		{"from", "o.occurred_at >= $%[1]d"}, {"to", "o.occurred_at < $%[1]d"},
	} {
		text, supplied := options.Values[filter.key]
		if !supplied {
			continue
		}
		var value any = text
		if filter.key == "from" || filter.key == "to" {
			boundary, _ := time.Parse(time.RFC3339Nano, text)
			// PostgreSQL timestamps have microsecond precision. Round upward to
			// retain inclusive from and exclusive to for nanosecond boundaries.
			if remainder := boundary.Nanosecond() % 1000; remainder != 0 {
				boundary = boundary.Add(time.Duration(1000-remainder) * time.Nanosecond)
			}
			value = boundary
		}
		args = append(args, value)
		query += " AND " + fmt.Sprintf(filter.expression, len(args))
	}
	return listRows(ctx, r.db, query, args, options, "o", "time", scanOperation)
}
