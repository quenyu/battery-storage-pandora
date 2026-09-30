package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const readTestID = "00000000-0000-4000-8000-00000000000a"
const readTestUpperID = "00000000-0000-4000-8000-00000000000b"

var errReadQueryCaptured = errors.New("read query captured")

type capturedReadQuery struct {
	query string
	args  []any
}

func (q *capturedReadQuery) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("unexpected single-row query")
}
func (q *capturedReadQuery) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	q.query, q.args = query, args
	return nil, errReadQueryCaptured
}

func TestHistoryCursorUsesTupleOrderAndUpperBoundary(t *testing.T) {
	last := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	upper := last.Add(time.Hour)
	f := model.ListOptions{Limit: 10, Scope: "scope", Cursor: &model.Cursor{Scope: "scope", Mode: "time", ID: readTestID, Time: last.Format(time.RFC3339Nano), UpperID: readTestUpperID, UpperTime: upper.Format(time.RFC3339Nano)}}
	q := &capturedReadQuery{}
	_, err := listRows(context.Background(), q, operationSelect+" WHERE o.type=$1", []any{"TAKE"}, f, "o", "time", scanOperation)
	if !errors.Is(err, errReadQueryCaptured) {
		t.Fatal(err)
	}
	if !strings.Contains(q.query, "(o.occurred_at,o.id) < ($2,$3)") || !strings.Contains(q.query, "(o.occurred_at,o.id) <= ($4,$5)") || !strings.Contains(q.query, "ORDER BY o.occurred_at DESC,o.id DESC") {
		t.Fatalf("history query loses timestamp/id continuation or upper boundary: %s", q.query)
	}
	want := []any{"TAKE", last, readTestID, upper, readTestUpperID, 11}
	if !reflect.DeepEqual(q.args, want) {
		t.Fatalf("bound arguments = %#v, want %#v", q.args, want)
	}
	// Cursor ordering may not be applied to a different kind of listing.
	q = &capturedReadQuery{}
	_, err = listRows(context.Background(), q, employeeSelect+" WHERE true", nil, f, "e", "id", scanEmployee)
	if err == nil || ClassifyError(err).Status != 400 || q.query != "" {
		t.Fatalf("cursor ordering mismatch reached SQL: %v", err)
	}
}
