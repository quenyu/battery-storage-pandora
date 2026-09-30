package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

const readTestID = "00000000-0000-4000-8000-00000000000a"
const readTestUpperID = "00000000-0000-4000-8000-00000000000b"

func TestReadFiltersValidateContract(t *testing.T) {
	tests := []struct {
		name, query, allowed string
		status               int
	}{
		{"default", "", "", 0},
		{"maximum limit", "limit=200", "", 0},
		{"zero limit", "limit=0", "", 400},
		{"excessive limit", "limit=201", "", 400},
		{"signed limit", "limit=%2B1", "", 400},
		{"fractional limit", "limit=1.5", "", 400},
		{"duplicate", "status=STORED&status=ISSUED", "status", 400},
		{"old offset", "offset=1", "", 400},
		{"unknown", "unknown=x", "", 400},
		{"empty", "status=", "status", 400},
		{"bad percent encoding", "status=%ZZ", "status", 400},
		{"old status", "status=stored", "status", 422},
		{"old type", "type=checkout", "type", 422},
		{"canonical address", "location=2.1.4", "location", 0},
		{"leading zero", "location=02.1.4", "location", 422},
		{"zero coordinate", "location=2.0.4", "location", 422},
		{"partial address", "location=2.1", "location", 422},
		{"address whitespace", "location=%202.1.4", "location", 422},
		{"unbounded coordinates", "location=99999999999999999999999.2.3", "location", 0},
		{"bad uuid", "battery_id=bad", "battery_id", 400},
		{"boolean spelling", "is_active=1", "is_active", 400},
		{"bad time", "from=yesterday", "from", 400},
		{"reversed range", "from=2026-09-30T12:00:00Z&to=2026-09-30T11:00:00Z", "from to", 422},
		{"empty range", "from=2026-09-30T12:00:00Z&to=2026-09-30T12:00:00Z", "from to", 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/batteries", nil)
			r.URL.RawQuery = tt.query
			f, err := parseFilters(r, tt.allowed)
			if tt.status == 0 {
				if err != nil {
					t.Fatal(err)
				}
				if tt.query == "" && f.limit != 50 {
					t.Fatalf("default limit = %d", f.limit)
				}
				return
			}
			if err == nil || classify(err).Status != tt.status {
				t.Fatalf("error = %v, want status %d", err, tt.status)
			}
		})
	}
}

func TestCursorBindsEndpointAndNormalizedFilters(t *testing.T) {
	path := "/api/operations"
	q := url.Values{"battery_id": {readTestID}, "from": {"2026-09-30T12:00:00Z"}, "type": {"TAKE"}, "limit": {"1"}}
	r := httptest.NewRequest("GET", path+"?"+q.Encode(), nil)
	f, err := parseFilters(r, "battery_id from type")
	if err != nil {
		t.Fatal(err)
	}
	c := pageCursor{Scope: f.scope, Mode: "time", ID: readTestID, Time: "2026-09-30T13:00:00Z", UpperID: readTestUpperID, UpperTime: "2026-09-30T14:00:00Z"}
	q.Set("cursor", encodeCursor(c))
	// Equivalent dates and UUID case must not invalidate the continuation.
	q.Set("from", "2026-09-30T05:00:00-07:00")
	q.Set("battery_id", strings.ToUpper(readTestID))
	q.Set("limit", "200")
	if _, err = parseFilters(httptest.NewRequest("GET", path+"?"+q.Encode(), nil), "battery_id from type"); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []struct{ path, key, value string }{{path, "type", "MOVE"}, {path, "from", "2026-09-30T11:00:00Z"}, {"/api/employees/" + readTestID + "/operations", "type", "TAKE"}} {
		copyQ := url.Values{}
		for key, value := range q {
			copyQ[key] = append([]string(nil), value...)
		}
		copyQ.Set(changed.key, changed.value)
		_, err := parseFilters(httptest.NewRequest("GET", changed.path+"?"+copyQ.Encode(), nil), "battery_id from type")
		if err == nil || classify(err).Status != 400 {
			t.Fatalf("changed endpoint/filter accepted: %+v, error %v", changed, err)
		}
	}
}

func signedReadTestCursor(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + hex.EncodeToString(sum[:])
}

func TestCursorRejectsMalformedAndMissingFields(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/employees", nil)
	f, err := parseFilters(r, "")
	if err != nil {
		t.Fatal(err)
	}
	valid := encodeCursor(pageCursor{Scope: f.scope, Mode: "id", ID: readTestID})
	badRaw := []string{
		`{}`, `null`,
		`{"scope":"` + f.scope + `","mode":"id"}`,
		`{"scope":"` + f.scope + `","id":"` + readTestID + `"}`,
		`{"mode":"id","id":"` + readTestID + `"}`,
		`{"scope":"` + f.scope + `","mode":"id","id":"` + readTestID + `","unknown":true}`,
		`{"scope":"` + f.scope + `","mode":"id","id":"` + readTestID + `","time":"2026-09-30T12:00:00Z"}`,
		`{"scope":"` + f.scope + `","mode":"id","id":"` + readTestID + `"} {}`,
	}
	bad := []string{"garbage", "%%%", valid[:len(valid)-1] + "x", strings.Repeat("a", 4097)}
	for _, raw := range badRaw {
		bad = append(bad, signedReadTestCursor(raw))
	}
	for i, token := range bad {
		r := httptest.NewRequest("GET", "/api/employees?"+url.Values{"cursor": {token}}.Encode(), nil)
		_, err := parseFilters(r, "")
		if err == nil || classify(err).Status != 400 {
			t.Fatalf("malformed cursor case %d accepted: %v", i, err)
		}
	}
}

func TestCursorChecksHistoryUpperBoundary(t *testing.T) {
	validTime := pageCursor{Scope: "scope", Mode: "time", ID: readTestID, Time: "2026-09-30T12:00:00Z", UpperID: readTestUpperID, UpperTime: "2026-09-30T12:00:00Z"}
	validVersion := pageCursor{Scope: "scope", Mode: "version", ID: readTestID, Version: 3, UpperVersion: 5}
	for _, c := range []pageCursor{validTime, validVersion} {
		got, err := decodeCursor(encodeCursor(c))
		if err != nil || got != c {
			t.Fatalf("valid history cursor: got %+v, error %v", got, err)
		}
	}
	tests := []struct {
		name   string
		cursor pageCursor
		change func(*pageCursor)
	}{
		{"missing upper time", validTime, func(c *pageCursor) { c.UpperTime = "" }},
		{"missing upper id", validTime, func(c *pageCursor) { c.UpperID = "" }},
		{"newer last time", validTime, func(c *pageCursor) { c.Time = "2026-09-30T12:00:01Z" }},
		{"reversed tied ids", validTime, func(c *pageCursor) { c.ID, c.UpperID = c.UpperID, c.ID }},
		{"missing upper version", validVersion, func(c *pageCursor) { c.UpperVersion = 0 }},
		{"missing last version", validVersion, func(c *pageCursor) { c.Version = 0 }},
		{"newer last version", validVersion, func(c *pageCursor) { c.Version = 6 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.cursor
			tt.change(&c)
			if _, err := decodeCursor(encodeCursor(c)); err == nil {
				t.Fatal("invalid upper boundary accepted")
			}
		})
	}
}

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
	f := filters{limit: 10, scope: "scope", cursor: &pageCursor{Scope: "scope", Mode: "time", ID: readTestID, Time: last.Format(time.RFC3339Nano), UpperID: readTestUpperID, UpperTime: upper.Format(time.RFC3339Nano)}}
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
	if err == nil || classify(err).Status != 400 || q.query != "" {
		t.Fatalf("cursor ordering mismatch reached SQL: %v", err)
	}
}
