package handler

import (
	"battery-storage-pandora/internal/model"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
				if tt.query == "" && f.Limit != 50 {
					t.Fatalf("default limit = %d", f.Limit)
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
	c := model.Cursor{Scope: f.Scope, Mode: "time", ID: readTestID, Time: "2026-09-30T13:00:00Z", UpperID: readTestUpperID, UpperTime: "2026-09-30T14:00:00Z"}
	q.Set("cursor", model.EncodeCursor(c))
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
	valid := model.EncodeCursor(model.Cursor{Scope: f.Scope, Mode: "id", ID: readTestID})
	badRaw := []string{
		`{}`, `null`,
		`{"scope":"` + f.Scope + `","mode":"id"}`,
		`{"scope":"` + f.Scope + `","id":"` + readTestID + `"}`,
		`{"mode":"id","id":"` + readTestID + `"}`,
		`{"scope":"` + f.Scope + `","mode":"id","id":"` + readTestID + `","unknown":true}`,
		`{"scope":"` + f.Scope + `","mode":"id","id":"` + readTestID + `","time":"2026-09-30T12:00:00Z"}`,
		`{"scope":"` + f.Scope + `","mode":"id","id":"` + readTestID + `"} {}`,
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
	validTime := model.Cursor{Scope: "scope", Mode: "time", ID: readTestID, Time: "2026-09-30T12:00:00Z", UpperID: readTestUpperID, UpperTime: "2026-09-30T12:00:00Z"}
	validVersion := model.Cursor{Scope: "scope", Mode: "version", ID: readTestID, Version: 3, UpperVersion: 5}
	for _, c := range []model.Cursor{validTime, validVersion} {
		got, err := model.DecodeCursor(model.EncodeCursor(c))
		if err != nil || got != c {
			t.Fatalf("valid history cursor: got %+v, error %v", got, err)
		}
	}
	tests := []struct {
		name   string
		cursor model.Cursor
		change func(*model.Cursor)
	}{
		{"missing upper time", validTime, func(c *model.Cursor) { c.UpperTime = "" }},
		{"missing upper id", validTime, func(c *model.Cursor) { c.UpperID = "" }},
		{"newer last time", validTime, func(c *model.Cursor) { c.Time = "2026-09-30T12:00:01Z" }},
		{"reversed tied ids", validTime, func(c *model.Cursor) { c.ID, c.UpperID = c.UpperID, c.ID }},
		{"missing upper version", validVersion, func(c *model.Cursor) { c.UpperVersion = 0 }},
		{"missing last version", validVersion, func(c *model.Cursor) { c.Version = 0 }},
		{"newer last version", validVersion, func(c *model.Cursor) { c.Version = 6 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.cursor
			tt.change(&c)
			if _, err := model.DecodeCursor(model.EncodeCursor(c)); err == nil {
				t.Fatal("invalid upper boundary accepted")
			}
		})
	}
}
