package repository

import (
	"testing"
	"time"
)

func TestPostgresTimeBoundaryPreservesRangeEdges(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"2026-10-01T12:00:00Z", "2026-10-01T12:00:00Z"},
		{"2026-10-01T12:00:00.123456Z", "2026-10-01T12:00:00.123456Z"},
		{"2026-10-01T12:00:00.123456001Z", "2026-10-01T12:00:00.123457Z"},
		{"2026-10-01T12:00:00.999999999Z", "2026-10-01T12:00:01Z"},
		{"2026-10-01T14:00:00.000000001+02:00", "2026-10-01T12:00:00.000001Z"},
	} {
		t.Run(test.input, func(t *testing.T) {
			want, err := time.Parse(time.RFC3339Nano, test.want)
			if err != nil {
				t.Fatal(err)
			}
			if got := postgresTimeBoundary(test.input); !got.Equal(want) {
				t.Fatalf("boundary = %s, want %s", got.Format(time.RFC3339Nano), test.want)
			}
		})
	}
}
