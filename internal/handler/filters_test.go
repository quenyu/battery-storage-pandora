package handler

import (
	"net/http/httptest"
	"testing"
)

func TestReadFiltersValidateContract(t *testing.T) {
	tests := []struct {
		name, query, allowed string
		status               int
	}{
		{"default", "", "", 0},
		{"removed limit", "limit=200", "", 400},
		{"removed cursor", "cursor=anything", "", 400},
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
			_, err := parseFilters(r, tt.allowed)
			if tt.status == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || classify(err).Status != tt.status {
				t.Fatalf("error = %v, want status %d", err, tt.status)
			}
		})
	}
}

func TestReadFiltersNormalizeValues(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/operations?employee_id=00000000-0000-4000-8000-00000000000A&from=2026-09-30T05:00:00-07:00&type=TAKE", nil)
	filters, err := parseFilters(r, "employee_id from type")
	if err != nil {
		t.Fatal(err)
	}
	if filters["employee_id"] != "00000000-0000-4000-8000-00000000000a" || filters["from"] != "2026-09-30T12:00:00Z" || filters["type"] != "TAKE" {
		t.Fatalf("unexpected normalized filters: %v", filters)
	}
}
