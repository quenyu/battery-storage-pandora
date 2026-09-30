package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"fmt"
	"time"
)

func listRows[T any](ctx context.Context, q queryer, query string, args []any, f model.ListOptions, alias, mode string, scan func(scanner) (T, error)) (model.Page, error) {
	c := model.Cursor{Scope: f.Scope, Mode: mode}
	if f.Cursor != nil {
		c = *f.Cursor
		if c.Mode != mode {
			return model.Page{}, model.Invalid("Неверный порядок курсора")
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
	args = append(args, f.Limit+1)
	query += fmt.Sprintf(" LIMIT $%d", len(args))
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return model.Page{}, err
	}
	defer rows.Close()
	items := make([]T, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return model.Page{}, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return model.Page{}, err
	}
	page := model.Page{Items: items}
	if len(items) > f.Limit {
		items = items[:f.Limit]
		page.Items = items
		if f.Cursor == nil {
			if o, ok := any(items[0]).(model.Operation); ok {
				if mode == "version" {
					c.UpperVersion = o.BatteryVersion
				} else {
					c.UpperTime = o.OccurredAt.Format(time.RFC3339Nano)
					c.UpperID = o.ID
				}
			}
		}
		switch last := any(items[len(items)-1]).(type) {
		case model.Employee:
			c.ID = last.ID
		case model.Credential:
			c.ID = last.ID
		case model.Battery:
			c.ID = last.ID
		case model.Operation:
			c.ID = last.ID
			if mode == "version" {
				c.Version = last.BatteryVersion
			} else {
				c.Time = last.OccurredAt.Format(time.RFC3339Nano)
			}
		}
		next := model.EncodeCursor(c)
		page.NextCursor = &next
	}
	return page, nil
}
