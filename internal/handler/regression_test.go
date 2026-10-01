package handler_test

import (
	"battery-storage-pandora/internal/handler"
	"battery-storage-pandora/internal/repository"
	"battery-storage-pandora/internal/service"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestIntegrationEmployeeDisablePreservesFirstTimestamp(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	type employeeStatus struct {
		IsActive   bool       `json:"is_active"`
		DisabledAt *time.Time `json:"disabled_at"`
	}
	path := "/employees/" + f.ivan.ID
	first := decode[employeeStatus](t, h.request("PATCH", path, map[string]any{"is_active": false}, uuid.NewString(), 200))
	again := decode[employeeStatus](t, h.request("PATCH", path, map[string]any{"is_active": false}, uuid.NewString(), 200))
	if first.IsActive || again.IsActive || first.DisabledAt == nil || again.DisabledAt == nil || !first.DisabledAt.Equal(*again.DisabledAt) {
		t.Fatalf("repeat disable changed activity or timestamp: first=%+v again=%+v", first, again)
	}
	enabled := decode[employeeStatus](t, h.request("PATCH", path, map[string]any{"is_active": true}, uuid.NewString(), 200))
	if !enabled.IsActive || enabled.DisabledAt != nil {
		t.Fatal("employee was not reactivated")
	}
}

func TestIntegrationDisabledCredentialCannotStore(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	h.request("PATCH", "/employees/"+f.ivan.ID+"/credentials/"+f.ivanCard.ID, map[string]any{"is_active": false}, uuid.NewString(), 200)
	errorCode(t, h.request("POST", "/batteries", storeBody("DISABLED-CARD", "1.1.1"), uuid.NewString(), 403), "CREDENTIAL_INACTIVE")
	if h.count("SELECT count(*) FROM batteries") != 0 || h.count("SELECT count(*) FROM battery_operations") != 0 {
		t.Fatal("disabled card changed batteries or history")
	}
}

func TestIntegrationConcurrentTakeAndDisableCredential(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	battery := h.register("CONCURRENT-CARD", "1.1.1")
	takePath := "/batteries/" + battery.Battery.ID + "/take"
	disablePath := "/employees/" + f.ivan.ID + "/credentials/" + f.ivanCard.ID
	results := h.parallel(
		commandJSON(t, "POST", takePath, uuid.NewString(), actionBody("00001234", "")),
		commandJSON(t, "PATCH", disablePath, uuid.NewString(), map[string]any{"is_active": false}),
	)
	h.check(results[1], 200)
	switch results[0].status {
	case 201:
		if h.battery(battery.Battery.ID).Version != 2 || h.count("SELECT count(*) FROM battery_operations WHERE type='TAKE'") != 1 {
			t.Fatal("successful TAKE did not commit once")
		}
	case 403:
		errorCode(t, results[0], "CREDENTIAL_INACTIVE")
		if h.battery(battery.Battery.ID).Version != 1 || h.count("SELECT count(*) FROM battery_operations WHERE type='TAKE'") != 0 {
			t.Fatal("rejected TAKE changed state")
		}
	default:
		t.Fatalf("concurrent TAKE returned %d: %s", results[0].status, results[0].body)
	}
}

func TestIntegrationTakeLocksEmployeeBeforeCredential(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	battery := h.register("LOCK-ORDER", "1.1.1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Pin the command to one PostgreSQL session so we can observe its blocked lock.
	db := stdlib.OpenDB(*h.config)
	db.SetMaxOpenConns(1)
	defer db.Close()
	var commandPID int
	if err := db.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&commandPID); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler.New(service.New(repository.New(db))))
	defer server.Close()

	// Simulate card revocation: hold the employee, then acquire the credential.
	blocker, err := h.owner.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	var blockerPID int
	if err := blocker.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := blocker.QueryRowContext(ctx, "SELECT id FROM employees WHERE id=$1 FOR UPDATE", f.ivan.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(actionBody("00001234", ""))
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		response response
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := perform(server.Client(), server.URL, "POST", "/batteries/"+battery.Battery.ID+"/take", string(body), uuid.NewString(), "application/json")
		done <- outcome{res, err}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		var waiting bool
		if err := h.owner.QueryRowContext(ctx, "SELECT $1::integer = ANY(pg_blocking_pids($2))", blockerPID, commandPID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not wait for the employee lock")
		}
		select {
		case result := <-done:
			t.Fatalf("TAKE finished before employee was released: %+v", result)
		case <-time.After(10 * time.Millisecond):
		}
	}
	// NOWAIT makes an inverted lock order fail immediately instead of timing out.
	if err := blocker.QueryRowContext(ctx, "SELECT id FROM employee_credentials WHERE id=$1 FOR UPDATE NOWAIT", f.ivanCard.ID).Scan(&id); err != nil {
		t.Fatalf("TAKE locked the credential before the employee: %v", err)
	}
	if _, err := blocker.ExecContext(ctx, "UPDATE employee_credentials SET disabled_at=clock_timestamp(), updated_at=clock_timestamp() WHERE id=$1", f.ivanCard.ID); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		h.check(result.response, 403)
		errorCode(t, result.response, "CREDENTIAL_INACTIVE")
	case <-ctx.Done():
		t.Fatal("TAKE did not finish after revocation")
	}
	if h.battery(battery.Battery.ID).Version != 1 || h.count("SELECT count(*) FROM battery_operations WHERE type='TAKE'") != 0 {
		t.Fatal("TAKE used a concurrently revoked credential")
	}
}
