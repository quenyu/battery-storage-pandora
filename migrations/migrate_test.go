package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func migrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL migration tests")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(config.Database, "_test") {
		t.Fatal("TEST_DATABASE_URL database must end in _test")
	}
	admin := stdlib.OpenDB(*config)
	t.Cleanup(func() { admin.Close() })
	schema := "migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec("CREATE SCHEMA " + quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + quoted + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { db.Close() })
	return db
}

func mustExecute(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func TestFreshSchemaHasTextAddresses(t *testing.T) {
	db := migrationDB(t)
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 6 {
		t.Fatalf("tables = %d, expected 5 application tables and migration metadata", tables)
	}
	var oldTables int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('cabinets','shelves','cells')`).Scan(&oldTables); err != nil {
		t.Fatal(err)
	}
	if oldTables != 0 {
		t.Fatal("location tables must not be created")
	}
	var textAddresses int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND data_type='text' AND ((table_name='batteries' AND column_name='current_location') OR (table_name='battery_operations' AND column_name IN ('source_location','destination_location')))`).Scan(&textAddresses); err != nil {
		t.Fatal(err)
	}
	if textAddresses != 3 {
		t.Fatal("all addresses must be text")
	}
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var versions int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 3 {
		t.Fatalf("migration count = %d", versions)
	}
}

const currentFixture = `
 INSERT INTO employees(id,display_name) VALUES ('00000000-0000-4000-8000-000000000001','Employee');
 INSERT INTO employee_credentials(id,employee_id,value) VALUES ('00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000001','000001');
 INSERT INTO batteries(id,inventory_code,status,current_location,version) VALUES ('00000000-0000-4000-8000-000000000021','АКБ-1','STORED','1.2.3',1);
 INSERT INTO idempotency_requests(scope,key,request_hash,http_status,response_body) VALUES ('pandora','00000000-0000-4000-8000-000000000031',repeat('a',64),201,'{"result":"preserve"}');
 INSERT INTO battery_operations(id,battery_id,battery_version,type,actor_employee_id,credential_id,destination_status,destination_location,request_scope,request_key) VALUES ('00000000-0000-4000-8000-000000000041','00000000-0000-4000-8000-000000000021',1,'STORE','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000011','STORED','1.2.3','pandora','00000000-0000-4000-8000-000000000031');
`

func snapshotCurrent(t *testing.T, db *sql.DB) string {
	t.Helper()
	var snapshot strings.Builder
	for _, table := range []string{"employees", "employee_credentials", "batteries", "battery_operations", "idempotency_requests"} {
		var rows string
		query := fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(row) ORDER BY to_jsonb(row)::text),'[]'::jsonb)::text FROM %s row`, table)
		if err := db.QueryRow(query).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		snapshot.WriteString(rows)
	}
	return snapshot.String()
}

func TestRepeatedMigrationPreservesExistingRows(t *testing.T) {
	db := migrationDB(t)
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	mustExecute(t, db, currentFixture)
	before := snapshotCurrent(t, db)
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if after := snapshotCurrent(t, db); after != before {
		t.Fatal("repeated migration changed stored rows")
	}
	var obsoleteGuards int
	if err := db.QueryRow(`SELECT count(*) FROM pg_constraint WHERE connamespace=current_schema()::regnamespace AND conname IN ('battery_position_ck','operation_shape_ck')`).Scan(&obsoleteGuards); err != nil {
		t.Fatal(err)
	}
	if obsoleteGuards != 0 {
		t.Fatal("obsolete shape checks are still present")
	}
}

func TestChecksumMismatchKeepsData(t *testing.T) {
	db := migrationDB(t)
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	mustExecute(t, db, currentFixture)
	before := snapshotCurrent(t, db)
	result, err := db.Exec(`UPDATE schema_migrations SET checksum='changed' WHERE version='001_schema.sql'`)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		t.Fatalf("checksum update affected %d rows: %v", count, err)
	}
	if err := Up(context.Background(), db); err == nil || !strings.Contains(err.Error(), "checksum changed") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	if after := snapshotCurrent(t, db); after != before {
		t.Fatal("failed migration changed data")
	}
}

func previousCredentialSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExecute(t, db, `CREATE TABLE schema_migrations (version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	for _, name := range []string{"001_schema.sql", "002_remove_old_guards.sql"} {
		body, err := files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		mustExecute(t, db, string(body))
		if _, err := db.Exec("INSERT INTO schema_migrations(version,checksum) VALUES ($1,$2)", name, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
			t.Fatal(err)
		}
	}
	mustExecute(t, db, currentFixture)
}

func TestSingleActiveCredentialMigrationPreservesHistory(t *testing.T) {
	db := migrationDB(t)
	previousCredentialSchema(t, db)
	mustExecute(t, db, `INSERT INTO employee_credentials(id,employee_id,value,disabled_at) VALUES ('00000000-0000-4000-8000-000000000012','00000000-0000-4000-8000-000000000001','000002',clock_timestamp())`)
	before := snapshotCurrent(t, db)
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if after := snapshotCurrent(t, db); after != before {
		t.Fatal("single card migration changed history or application data")
	}
	if _, err := db.Exec(`INSERT INTO employee_credentials(id,employee_id,value) VALUES ('00000000-0000-4000-8000-000000000013','00000000-0000-4000-8000-000000000001','000003')`); err == nil {
		t.Fatal("upgraded schema allowed a second active card")
	}
}

func TestSingleActiveCredentialMigrationRejectsDuplicates(t *testing.T) {
	db := migrationDB(t)
	previousCredentialSchema(t, db)
	mustExecute(t, db, `INSERT INTO employee_credentials(id,employee_id,value) VALUES ('00000000-0000-4000-8000-000000000012','00000000-0000-4000-8000-000000000001','000002')`)
	before := snapshotCurrent(t, db)
	if err := Up(context.Background(), db); err == nil || !strings.Contains(err.Error(), "003_single_active_credential.sql") {
		t.Fatalf("expected duplicate cards to stop migration, got %v", err)
	}
	if after := snapshotCurrent(t, db); after != before {
		t.Fatal("failed migration changed existing cards or history")
	}
	var applied int
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version='003_single_active_credential.sql'").Scan(&applied); err != nil || applied != 0 {
		t.Fatalf("failed migration was recorded as applied: %d, %v", applied, err)
	}
	mustExecute(t, db, `UPDATE employee_credentials SET disabled_at=clock_timestamp() WHERE id='00000000-0000-4000-8000-000000000012'`)
	if err := Up(context.Background(), db); err != nil {
		t.Fatal("migration failed after resolving duplicates:", err)
	}
}
