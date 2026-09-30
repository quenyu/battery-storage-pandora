package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Every test owns a disposable schema in a dedicated *_test database. Never
// apply migrations to the existing demonstration database while testing.
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

func execMigrationSQL(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatal(err)
	}
}

func installLegacy(t *testing.T, db *sql.DB) {
	t.Helper()
	body, err := files.ReadFile("001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	execMigrationSQL(t, db, string(body))
	execMigrationSQL(t, db, `CREATE TABLE schema_migrations(version text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	execMigrationSQL(t, db, `INSERT INTO schema_migrations(version,checksum) VALUES('001_initial.sql',$1)`, fmt.Sprintf("%x", sha256.Sum256(body)))
	execMigrationSQL(t, db, legacyFixture)
}

const legacyFixture = `
INSERT INTO employees(id,name,created_at) VALUES
 ('00000000-0000-4000-8000-000000000001','Employee One','2026-09-01Z'),
 ('00000000-0000-4000-8000-000000000002','Employee Two','2026-09-01Z');
INSERT INTO employee_credentials(id,employee_id,barcode,created_at,revoked_at) VALUES
 ('00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000001','000001','2026-09-01Z','2026-09-04Z'),
 ('00000000-0000-4000-8000-000000000012','00000000-0000-4000-8000-000000000001','000002','2026-09-04Z',NULL),
 ('00000000-0000-4000-8000-000000000013','00000000-0000-4000-8000-000000000002','000003','2026-09-01Z',NULL);
INSERT INTO cabinets(id,number) VALUES ('00000000-0000-4000-8000-000000000021',12);
INSERT INTO shelves(id,cabinet_id,number) VALUES
 ('00000000-0000-4000-8000-000000000022','00000000-0000-4000-8000-000000000021',34);
INSERT INTO cells(id,shelf_id,number) VALUES
 ('00000000-0000-4000-8000-000000000023','00000000-0000-4000-8000-000000000022',56),
 ('00000000-0000-4000-8000-000000000024','00000000-0000-4000-8000-000000000022',57),
 ('00000000-0000-4000-8000-000000000025','00000000-0000-4000-8000-000000000022',58);
INSERT INTO batteries(id,inventory_code,status,cell_id,holder_employee_id,version,created_at,updated_at) VALUES
 ('00000000-0000-4000-8000-000000000031','B-0001','stored','00000000-0000-4000-8000-000000000025',NULL,4,'2026-09-02Z','2026-09-05Z'),
 ('00000000-0000-4000-8000-000000000032','B-0002','issued',NULL,'00000000-0000-4000-8000-000000000001',2,'2026-09-06Z','2026-09-07Z');
INSERT INTO operations(id,battery_id,battery_version,type,actor_employee_id,credential_id,from_cell_id,to_cell_id,from_holder_employee_id,to_holder_employee_id,occurred_at) VALUES
 ('00000000-0000-4000-8000-000000000041','00000000-0000-4000-8000-000000000031',1,'register','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000011',NULL,'00000000-0000-4000-8000-000000000023',NULL,NULL,'2026-09-02Z'),
 ('00000000-0000-4000-8000-000000000042','00000000-0000-4000-8000-000000000031',2,'checkout','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000023',NULL,NULL,'00000000-0000-4000-8000-000000000001','2026-09-03Z'),
 ('00000000-0000-4000-8000-000000000043','00000000-0000-4000-8000-000000000031',3,'return','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000012',NULL,'00000000-0000-4000-8000-000000000024','00000000-0000-4000-8000-000000000001',NULL,'2026-09-04Z'),
 ('00000000-0000-4000-8000-000000000044','00000000-0000-4000-8000-000000000031',4,'move','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000012','00000000-0000-4000-8000-000000000024','00000000-0000-4000-8000-000000000025',NULL,NULL,'2026-09-05Z'),
 ('00000000-0000-4000-8000-000000000045','00000000-0000-4000-8000-000000000032',1,'register','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000012',NULL,'00000000-0000-4000-8000-000000000023',NULL,NULL,'2026-09-06Z'),
 ('00000000-0000-4000-8000-000000000046','00000000-0000-4000-8000-000000000032',2,'checkout','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000012','00000000-0000-4000-8000-000000000023',NULL,NULL,'00000000-0000-4000-8000-000000000001','2026-09-07Z');
INSERT INTO idempotency_records(key,request_hash,http_status,response_body,created_at) VALUES
 ('old-key',repeat('a',64),201,'{"old":"original response"}','2026-09-02Z');
`

var archivedTables = []string{"employees", "employee_credentials", "cabinets", "shelves", "cells", "batteries", "operations", "idempotency_records"}

func snapshotLegacy(t *testing.T, db *sql.DB, prefix string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, name := range archivedTables {
		var snapshot string
		query := "SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text),'[]'::jsonb)::text FROM " + pgx.Identifier{prefix + name}.Sanitize() + " x"
		if err := db.QueryRow(query).Scan(&snapshot); err != nil {
			t.Fatal(err)
		}
		result[name] = snapshot
	}
	return result
}

func assertMigrationBool(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	var ok bool
	if err := db.QueryRow(query).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("migration assertion failed: %s", query)
	}
}

func TestMigrationTransfersLocationsHistoryAndCredentials(t *testing.T) {
	db := migrationDB(t)
	installLegacy(t, db)
	before := snapshotLegacy(t, db, "")
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if after := snapshotLegacy(t, db, "legacy_"); !reflect.DeepEqual(before, after) {
		t.Fatal("archived legacy data changed")
	}
	assertMigrationBool(t, db, `SELECT (SELECT count(*) FROM employees)=2 AND (SELECT count(*) FROM employee_credentials)=3 AND (SELECT count(*) FROM batteries)=2 AND (SELECT count(*) FROM battery_operations)=6 AND (SELECT count(*) FROM idempotency_requests)=6`)
	assertMigrationBool(t, db, `SELECT current_location='12.34.58' AND status='STORED' AND version=4 AND current_holder_employee_id IS NULL AND created_at='2026-09-02Z' AND updated_at='2026-09-05Z' FROM batteries WHERE inventory_code='B-0001'`)
	assertMigrationBool(t, db, `SELECT current_location IS NULL AND status='ISSUED' AND version=2 AND current_holder_employee_id='00000000-0000-4000-8000-000000000001' FROM batteries WHERE inventory_code='B-0002'`)
	assertMigrationBool(t, db, `SELECT display_name='Employee One' AND created_at=updated_at AND disabled_at IS NULL FROM employees WHERE id='00000000-0000-4000-8000-000000000001'`)
	assertMigrationBool(t, db, `SELECT value='000001' AND disabled_at='2026-09-04Z' AND updated_at=disabled_at AND created_at='2026-09-01Z' FROM employee_credentials WHERE id='00000000-0000-4000-8000-000000000011'`)
	assertMigrationBool(t, db, `SELECT bool_and(n.id=o.id AND n.credential_id=o.credential_id AND n.actor_employee_id=o.actor_employee_id AND n.occurred_at=o.occurred_at AND n.battery_version=o.battery_version AND n.source_holder_employee_id IS NOT DISTINCT FROM o.from_holder_employee_id AND n.destination_holder_employee_id IS NOT DISTINCT FROM o.to_holder_employee_id) FROM battery_operations n JOIN legacy_operations o ON n.id=o.id`)
	assertMigrationBool(t, db, `SELECT array_agg(type ORDER BY battery_version)=ARRAY['STORE','TAKE','RETURN','MOVE'] AND array_agg(COALESCE(source_location,'-') ORDER BY battery_version)=ARRAY['-','12.34.56','-','12.34.57'] AND array_agg(COALESCE(destination_location,'-') ORDER BY battery_version)=ARRAY['12.34.56','-','12.34.57','12.34.58'] FROM battery_operations WHERE battery_id='00000000-0000-4000-8000-000000000031'`)
	assertMigrationBool(t, db, `SELECT bool_and(scope='legacy-migration' AND http_status=201 AND response_body IS NOT NULL) FROM idempotency_requests`)
	assertMigrationBool(t, db, `SELECT NOT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname=current_schema() AND tablename LIKE 'legacy_%' AND indexname NOT LIKE 'legacy_%')`)
	assertMigrationBool(t, db, `SELECT to_regclass('cells') IS NULL AND to_regclass('shelves') IS NULL AND to_regclass('cabinets') IS NULL`)
	if err := Up(context.Background(), db); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	assertMigrationBool(t, db, `SELECT count(*)=2 FROM schema_migrations`)
	if after := snapshotLegacy(t, db, "legacy_"); !reflect.DeepEqual(before, after) {
		t.Fatal("second migration changed archived data")
	}
}

func TestMigrationRejectsIncompatibleLegacyDataAtomically(t *testing.T) {
	tests := []struct{ name, mutation, message string }{
		{"foreign_return", `UPDATE operations SET actor_employee_id='00000000-0000-4000-8000-000000000002',credential_id='00000000-0000-4000-8000-000000000013' WHERE type='return'`, "RETURN actor differs"},
		{"loss", `INSERT INTO operations(id,battery_id,battery_version,type,actor_employee_id,credential_id,from_cell_id,reason,occurred_at) VALUES('00000000-0000-4000-8000-000000000047','00000000-0000-4000-8000-000000000031',5,'loss','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000012','00000000-0000-4000-8000-000000000025','lost','2026-09-08Z'); UPDATE batteries SET status='lost',cell_id=NULL,version=5,updated_at='2026-09-08Z' WHERE inventory_code='B-0001'`, "LOST/loss"},
		{"wrong_source", `UPDATE operations SET from_cell_id='00000000-0000-4000-8000-000000000023' WHERE type='move'`, "valid version chain"},
		{"gap", `DELETE FROM operations WHERE id='00000000-0000-4000-8000-000000000042'`, "complete history"},
		{"projection", `UPDATE batteries SET updated_at='2026-09-09Z' WHERE inventory_code='B-0001'`, "complete history"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := migrationDB(t)
			installLegacy(t, db)
			execMigrationSQL(t, db, test.mutation)
			before := snapshotLegacy(t, db, "")
			err := Up(context.Background(), db)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected clear migration refusal containing %q, got %v", test.message, err)
			}
			if after := snapshotLegacy(t, db, ""); !reflect.DeepEqual(before, after) {
				t.Fatal("refused migration modified source data")
			}
			assertMigrationBool(t, db, `SELECT to_regclass('legacy_batteries') IS NULL AND to_regclass('battery_operations') IS NULL AND (SELECT count(*) FROM schema_migrations)=1`)
		})
	}
}

func TestFreshAndRepeatedMigration(t *testing.T) {
	db := migrationDB(t)
	for i := 0; i < 2; i++ {
		if err := Up(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	assertMigrationBool(t, db, `SELECT (SELECT count(*) FROM schema_migrations)=2 AND (SELECT count(*) FROM battery_operations)=0 AND (SELECT count(*) FROM legacy_operations)=0`)
}

func TestMigrationFailureAfterRenamesRollsBackDDLAndData(t *testing.T) {
	db := migrationDB(t)
	installLegacy(t, db)
	execMigrationSQL(t, db, `CREATE TABLE legacy_batteries(conflict text)`)
	before := snapshotLegacy(t, db, "")
	if err := Up(context.Background(), db); err == nil {
		t.Fatal("expected archive table name collision to fail")
	}
	if after := snapshotLegacy(t, db, ""); !reflect.DeepEqual(before, after) {
		t.Fatal("failed migration modified legacy data")
	}
	assertMigrationBool(t, db, `SELECT to_regclass('legacy_employees') IS NULL AND to_regclass('employees_pkey') IS NOT NULL AND to_regclass('legacy_employees_pkey') IS NULL AND (SELECT count(*) FROM schema_migrations)=1`)
}

func TestMigratedSchemaEnforcesGuards(t *testing.T) {
	db := migrationDB(t)
	installLegacy(t, db)
	if err := Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE battery_operations SET occurred_at=clock_timestamp()`,
		`DELETE FROM battery_operations`,
		`TRUNCATE battery_operations`,
		`UPDATE employee_credentials SET value='different'`,
		`UPDATE batteries SET inventory_code='different'`,
		`UPDATE idempotency_requests SET response_body='{}'`,
		`INSERT INTO idempotency_requests(scope,key,request_hash) VALUES('test','00000000-0000-4000-8000-000000000099',repeat('a',64))`,
		`UPDATE batteries SET version=version+1 WHERE inventory_code='B-0001'`,
		`UPDATE legacy_operations SET occurred_at=clock_timestamp()`,
		`INSERT INTO legacy_idempotency_records(key,request_hash) VALUES('new-legacy-key',repeat('b',64))`,
		`TRUNCATE legacy_idempotency_records`,
	} {
		if _, err := db.Exec(query); err == nil {
			t.Fatalf("database guard accepted invalid mutation: %s", query)
		}
	}
	for _, address := range []string{"01.2.3", "1.02.3", "1.2.03", "0.2.3", "1.0.3", "1.2.0", "1.2", "1.2.3.4", "1.2.3 ", " 1.2.3", "A.2.3"} {
		_, err := db.Exec(`UPDATE batteries SET current_location=$1 WHERE inventory_code='B-0001'`, address)
		if err == nil || !strings.Contains(err.Error(), "batteries_current_location_check") {
			t.Fatalf("address %q should fail canonical location CHECK before projection guard, got %v", address, err)
		}
	}
	assertMigrationBool(t, db, `SELECT (SELECT count(*) FROM battery_operations)=6 AND (SELECT count(*) FROM idempotency_requests)=6 AND (SELECT version FROM batteries WHERE inventory_code='B-0001')=4`)
}
