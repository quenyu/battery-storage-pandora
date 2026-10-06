package service_test

import (
	"battery-storage-pandora/internal/database"
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"battery-storage-pandora/internal/service"
	"battery-storage-pandora/migrations"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Integration tests need the local test database:
//
//	TEST_DATABASE_URL=postgres://pandora_owner@127.0.0.1:55432/pandora_test?sslmode=disable
//	TEST_APP_DATABASE_URL=postgres://pandora_app@127.0.0.1:55432/pandora_test?sslmode=disable
//
// Every test migrates its own schema and applies scripts/postgres-grants.sql to it,
// then works through the application role exactly like the service does.
type testDB struct {
	owner *sql.DB
	app   *sql.DB
}

func newTestDB(t *testing.T) testDB {
	t.Helper()
	ownerURL, appURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_APP_DATABASE_URL")
	if ownerURL == "" || appURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_APP_DATABASE_URL are not set")
	}
	ctx := context.Background()
	schema := "t_" + strings.ToLower(rand.Text()[:12])
	admin := open(t, ownerURL)
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`) })

	db := testDB{owner: open(t, withSchema(ownerURL, schema)), app: open(t, withSchema(appURL, schema))}
	if err := migrations.Up(ctx, db.owner); err != nil {
		t.Fatal(err)
	}
	grants, err := os.ReadFile("../../scripts/postgres-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.owner.ExecContext(ctx, strings.ReplaceAll(string(grants), "SCHEMA public", "SCHEMA "+schema)); err != nil {
		t.Fatal(err)
	}
	return db
}

func open(t *testing.T, url string) *sql.DB {
	t.Helper()
	db, err := database.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func withSchema(url, schema string) string {
	separator := "?"
	if strings.Contains(url, "?") {
		separator = "&"
	}
	return url + separator + "search_path=" + schema
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	return service.ClassifyError(err).Code
}

func decode[T any](t *testing.T, response model.CommandResponse, err error, status int) T {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.Status != status {
		t.Fatalf("status %d, want %d", response.Status, status)
	}
	var value T
	if err := json.Unmarshal(response.Body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func addEmployee(t *testing.T, s *service.Service, name, card string) model.Employee {
	t.Helper()
	ctx := context.Background()
	response, err := s.CreateEmployee(ctx, model.CreateEmployeeInput{DisplayName: name})
	employee := decode[model.Employee](t, response, err, 201)
	response, err = s.CreateCredential(ctx, employee.ID, model.CreateCredentialInput{Value: card})
	decode[model.Credential](t, response, err, 201)
	return employee
}

func store(s *service.Service, code, card, location string) (model.CommandResponse, error) {
	return s.RegisterBattery(context.Background(), model.RegisterBatteryInput{
		InventoryCode: code, ActorCredential: card, DestinationLocation: location,
	})
}

func command(card, location string) model.BatteryCommandInput {
	return model.BatteryCommandInput{ActorCredential: card, DestinationLocation: location}
}

func TestMigrationsAreRepeatableAndChecked(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := migrations.Up(ctx, db.owner); err != nil {
		t.Fatalf("repeated migration: %v", err)
	}
	if _, err := db.owner.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'changed'`); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(ctx, db.owner); err == nil || !strings.Contains(err.Error(), "checksum changed") {
		t.Fatalf("changed checksum must fail, got %v", err)
	}
}

func TestBatteryLifecycle(t *testing.T) {
	db := newTestDB(t)
	s := service.New(repository.New(db.app))
	ctx := context.Background()
	alice := addEmployee(t, s, "Alice", "CARD-A")
	bob := addEmployee(t, s, "Bob", "CARD-B")

	response, err := store(s, "AKB-1", "CARD-A", "1.1.1")
	stored := decode[model.CommandResult](t, response, err, 201)
	if stored.Battery.Status != model.BatteryStored || *stored.Battery.CurrentLocation != "1.1.1" {
		t.Fatalf("unexpected state after STORE: %+v", stored.Battery)
	}
	response, err = store(s, "AKB-1", "CARD-A", "1.1.1")
	if replay := decode[model.CommandResult](t, response, err, 200); replay.Operation.ID != stored.Operation.ID {
		t.Fatal("repeated STORE must return the original operation")
	}
	if _, err := store(s, "AKB-1", "CARD-B", "1.1.1"); errorCode(err) != "INVENTORY_CODE_EXISTS" {
		t.Fatalf("STORE by another card: %v", err)
	}

	response, err = s.TakeBattery(ctx, "AKB-1", command("CARD-A", ""))
	taken := decode[model.CommandResult](t, response, err, 201)
	if taken.Battery.Status != model.BatteryIssued || *taken.Battery.CurrentHolderEmployeeID != alice.ID ||
		taken.Battery.CurrentLocation != nil || *taken.Operation.SourceLocation != "1.1.1" {
		t.Fatalf("unexpected TAKE result: %+v", taken)
	}
	response, err = s.TakeBattery(ctx, "AKB-1", command("CARD-A", ""))
	decode[model.CommandResult](t, response, err, 200)
	if _, err := s.TakeBattery(ctx, "AKB-1", command("CARD-B", "")); errorCode(err) != "BATTERY_ALREADY_ISSUED" {
		t.Fatalf("TAKE by another employee: %v", err)
	}
	if _, err := s.ReturnBattery(ctx, "AKB-1", command("CARD-B", "1.1.2")); errorCode(err) != "RETURN_NOT_ALLOWED" {
		t.Fatalf("RETURN by another employee: %v", err)
	}

	response, err = s.ReturnBattery(ctx, "AKB-1", command("CARD-A", "1.1.2"))
	decode[model.CommandResult](t, response, err, 201)
	response, err = s.MoveBattery(ctx, "AKB-1", command("CARD-B", "1.1.3"))
	moved := decode[model.CommandResult](t, response, err, 201)
	response, err = s.MoveBattery(ctx, "AKB-1", command("CARD-B", "1.1.3"))
	decode[model.CommandResult](t, response, err, 200)
	if _, err := s.MoveBattery(ctx, "AKB-1", command("CARD-A", "1.1.3")); errorCode(err) != "SAME_LOCATION" {
		t.Fatalf("same MOVE with another card is not a replay: %v", err)
	}

	history, err := s.ListBatteryOperations(ctx, moved.Battery.ID)
	if err != nil {
		t.Fatal(err)
	}
	var journal []string
	for _, operation := range history {
		journal = append(journal, fmt.Sprintf("%s %d %v->%v", operation.Type, operation.ActorEmployeeID,
			text(operation.SourceLocation), text(operation.DestinationLocation)))
	}
	want := []string{
		fmt.Sprintf("MOVE %d 1.1.2->1.1.3", bob.ID),
		fmt.Sprintf("RETURN %d -->1.1.2", alice.ID),
		fmt.Sprintf("TAKE %d 1.1.1->-", alice.ID),
		fmt.Sprintf("STORE %d -->1.1.1", alice.ID),
	}
	if strings.Join(journal, "; ") != strings.Join(want, "; ") {
		t.Fatalf("history:\n%v\nwant:\n%v", journal, want)
	}

	// The location filter sees both sides of a move; the employee filter sees the actor.
	byLocation, err := s.ListOperations(ctx, model.Filters{"location": "1.1.2"})
	if err != nil || len(byLocation) != 2 {
		t.Fatalf("location filter: %d operations, %v", len(byLocation), err)
	}
	byEmployee, err := s.ListEmployeeOperations(ctx, bob.ID)
	if err != nil || len(byEmployee) != 1 {
		t.Fatalf("employee filter: %d operations, %v", len(byEmployee), err)
	}

	// Only the current state occupies a location: 1.1.1 was vacated, 1.1.3 is taken.
	if _, err := store(s, "AKB-2", "CARD-A", "1.1.3"); errorCode(err) != "LOCATION_OCCUPIED" {
		t.Fatalf("STORE into occupied location: %v", err)
	}
	response, err = store(s, "AKB-2", "CARD-A", "1.1.1")
	decode[model.CommandResult](t, response, err, 201)
}

func TestCredentialReplacement(t *testing.T) {
	db := newTestDB(t)
	s := service.New(repository.New(db.app))
	ctx := context.Background()
	alice := addEmployee(t, s, "Alice", "CARD-OLD")
	credentials, err := s.ListCredentials(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	response, err := s.CreateCredential(ctx, alice.ID, model.CreateCredentialInput{
		Value: "CARD-NEW", ReplacesCredentialID: &credentials[0].ID,
	})
	decode[model.Credential](t, response, err, 201)
	if _, err := store(s, "AKB-1", "CARD-OLD", "1.1.1"); errorCode(err) != "CREDENTIAL_INACTIVE" {
		t.Fatalf("old card: %v", err)
	}
	response, err = store(s, "AKB-1", "CARD-NEW", "1.1.1")
	decode[model.CommandResult](t, response, err, 201)
}

func TestHistoryIsAppendOnlyForApplication(t *testing.T) {
	db := newTestDB(t)
	s := service.New(repository.New(db.app))
	addEmployee(t, s, "Alice", "CARD-A")
	response, err := store(s, "AKB-1", "CARD-A", "1.1.1")
	decode[model.CommandResult](t, response, err, 201)
	for _, statement := range []string{
		`UPDATE battery_operations SET location = '9.9.9'`,
		`DELETE FROM battery_operations`,
		`UPDATE batteries SET inventory_code = 'X', serial_number = 'X'`,
		`UPDATE batteries SET created_at = now()`,
		`DELETE FROM batteries`,
		// A card never changes owner or value, so the actor of past operations is fixed.
		`UPDATE employee_credentials SET employee_id = employee_id`,
		`UPDATE employee_credentials SET value = 'OTHER'`,
		`DELETE FROM employee_credentials`,
		`UPDATE employees SET id = id`,
		`UPDATE employees SET personnel_number = 'X'`,
	} {
		_, err := db.app.ExecContext(context.Background(), statement)
		var pgError *pgconn.PgError
		if !errors.As(err, &pgError) || pgError.Code != "42501" {
			t.Errorf("%s: expected permission denied, got %v", statement, err)
		}
	}
}

func TestConcurrentTakeOfOneBattery(t *testing.T) {
	db := newTestDB(t)
	s := service.New(repository.New(db.app))
	const workers = 10
	for i := range workers {
		addEmployee(t, s, fmt.Sprint("Worker ", i), fmt.Sprint("CARD-", i))
	}
	response, err := store(s, "AKB-1", "CARD-0", "1.1.1")
	decode[model.CommandResult](t, response, err, 201)

	created, conflicts := runConcurrently(workers, func(i int) (model.CommandResponse, error) {
		return s.TakeBattery(context.Background(), "AKB-1", command(fmt.Sprint("CARD-", i), ""))
	}, "BATTERY_ALREADY_ISSUED")
	if created != 1 || conflicts != workers-1 {
		t.Fatalf("created %d, conflicts %d", created, conflicts)
	}
}

func TestConcurrentMovesIntoOneLocation(t *testing.T) {
	db := newTestDB(t)
	s := service.New(repository.New(db.app))
	addEmployee(t, s, "Alice", "CARD-A")
	const batteries = 10
	for i := range batteries {
		response, err := store(s, fmt.Sprint("AKB-", i), "CARD-A", fmt.Sprintf("1.1.%d", i+1))
		decode[model.CommandResult](t, response, err, 201)
	}

	created, conflicts := runConcurrently(batteries, func(i int) (model.CommandResponse, error) {
		return s.MoveBattery(context.Background(), fmt.Sprint("AKB-", i), command("CARD-A", "2.2.2"))
	}, "LOCATION_OCCUPIED")
	if created != 1 || conflicts != batteries-1 {
		t.Fatalf("created %d, conflicts %d", created, conflicts)
	}
}

func runConcurrently(n int, run func(int) (model.CommandResponse, error), conflictCode string) (created, conflicts int) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			response, err := run(i)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && response.Status == 201:
				created++
			case errorCode(err) == conflictCode:
				conflicts++
			}
		})
	}
	wg.Wait()
	return created, conflicts
}

func text(value *string) string {
	if value == nil {
		return "-"
	}
	return *value
}
