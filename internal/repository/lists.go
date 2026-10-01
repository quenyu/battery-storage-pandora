package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"fmt"
	"time"
)

func (r *Repository) ListEmployees(ctx context.Context, filters model.Filters) ([]model.Employee, error) {
	query := employeeSelect + ` WHERE true`
	if active, supplied := filters["is_active"]; supplied {
		if active == "true" {
			query += ` AND e.disabled_at IS NULL`
		} else {
			query += ` AND e.disabled_at IS NOT NULL`
		}
	}
	query += ` ORDER BY e.id`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	employees := []model.Employee{}
	for rows.Next() {
		employee, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		employees = append(employees, employee)
	}
	return employees, rows.Err()
}

func (r *Repository) ListCredentials(ctx context.Context, employeeID string) ([]model.Credential, error) {
	query := credentialSelect + ` WHERE c.employee_id = $1 ORDER BY c.id`
	rows, err := r.db.QueryContext(ctx, query, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	credentials := []model.Credential{}
	for rows.Next() {
		credential, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, rows.Err()
}

func (r *Repository) ListBatteries(ctx context.Context, filters model.Filters) ([]model.Battery, error) {
	query := batterySelect + ` WHERE true`
	args := []any{}
	for _, filter := range []struct{ key, column string }{
		{"inventory_code", "inventory_code"}, {"status", "status"},
		{"holder_employee_id", "current_holder_employee_id"}, {"location", "current_location"},
	} {
		if value, supplied := filters[filter.key]; supplied {
			args = append(args, value)
			query += fmt.Sprintf(" AND b.%s = $%d", filter.column, len(args))
		}
	}
	query += ` ORDER BY b.id`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batteries := []model.Battery{}
	for rows.Next() {
		battery, err := scanBattery(rows)
		if err != nil {
			return nil, err
		}
		batteries = append(batteries, battery)
	}
	return batteries, rows.Err()
}

func (r *Repository) ListCustody(ctx context.Context, employeeID string) ([]model.Battery, error) {
	return r.ListBatteries(ctx, model.Filters{"holder_employee_id": employeeID})
}

func (r *Repository) ListBatteryOperations(ctx context.Context, batteryID string) ([]model.Operation, error) {
	query := operationSelect + ` WHERE o.battery_id = $1 ORDER BY o.battery_version DESC`
	return r.queryOperations(ctx, query, batteryID)
}

func (r *Repository) ListEmployeeOperations(ctx context.Context, employeeID string) ([]model.Operation, error) {
	query := operationSelect + `
        WHERE o.actor_employee_id = $1 OR o.source_holder_employee_id = $1 OR o.destination_holder_employee_id = $1
        ORDER BY o.occurred_at DESC, o.id DESC
    `
	return r.queryOperations(ctx, query, employeeID)
}

func (r *Repository) ListOperations(ctx context.Context, filters model.Filters) ([]model.Operation, error) {
	query := operationSelect + ` WHERE true`
	args := []any{}
	for _, filter := range []struct{ key, expression string }{
		{"battery_id", "o.battery_id=$%[1]d"},
		{"employee_id", "(o.actor_employee_id=$%[1]d OR o.source_holder_employee_id=$%[1]d OR o.destination_holder_employee_id=$%[1]d)"},
		{"location", "(o.source_location=$%[1]d OR o.destination_location=$%[1]d)"},
		{"device_code", "o.device_code=$%[1]d"}, {"type", "o.type=$%[1]d"},
		{"from", "o.occurred_at >= $%[1]d"}, {"to", "o.occurred_at < $%[1]d"},
	} {
		text, supplied := filters[filter.key]
		if !supplied {
			continue
		}
		var value any = text
		if filter.key == "from" || filter.key == "to" {
			value = postgresTimeBoundary(text)
		}
		args = append(args, value)
		query += " AND " + fmt.Sprintf(filter.expression, len(args))
	}
	query += ` ORDER BY o.occurred_at DESC, o.id DESC`
	return r.queryOperations(ctx, query, args...)
}

// Filters are validated by the HTTP layer. PostgreSQL timestamps have microsecond
// precision; rounding up preserves both the inclusive from and exclusive to bounds.
func postgresTimeBoundary(text string) time.Time {
	boundary, _ := time.Parse(time.RFC3339Nano, text)
	if remainder := boundary.Nanosecond() % 1000; remainder != 0 {
		boundary = boundary.Add(time.Duration(1000-remainder) * time.Nanosecond)
	}
	return boundary
}

func (r *Repository) queryOperations(ctx context.Context, query string, args ...any) ([]model.Operation, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := []model.Operation{}
	for rows.Next() {
		operation, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}
