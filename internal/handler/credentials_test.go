package handler_test

import (
	"errors"
	"testing"

	"battery-storage-pandora/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIntegrationSingleActiveCredentialLifecycle(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	path := "/employees/" + f.ivan.ID + "/credentials"
	newCardBody := map[string]any{"value": "00005678"}
	conflictKey := uuid.NewString()
	conflict := h.request("POST", path, newCardBody, conflictKey, 409)
	errorCode(t, conflict, "EMPLOYEE_ACTIVE_CREDENTIAL_EXISTS")
	equalJSON(t, conflict, h.request("POST", path, newCardBody, conflictKey, 409))

	// A foreign card cannot be revoked while replacing this employee's card.
	errorCode(t, h.request("POST", path, map[string]any{
		"value": "00005678", "replaces_credential_id": f.olgaCard.ID,
	}, uuid.NewString(), 404), "RESOURCE_NOT_FOUND")
	h.request("POST", "/credential-resolutions", map[string]any{"credential_value": f.olgaCard.Value}, "", 200)

	key := uuid.NewString()
	replacementBody := map[string]any{"value": "00005678", "replaces_credential_id": f.ivanCard.ID}
	replacement := h.request("POST", path, replacementBody, key, 201)
	newCard := decode[credentialDTO](t, replacement)
	if newCard.Value != "00005678" || newCard.EmployeeID != f.ivan.ID || !newCard.IsActive {
		t.Fatal("replacement changed barcode or employee identity")
	}
	replay := h.request("POST", path, replacementBody, key, 201)
	equalJSON(t, replacement, replay)
	if replay.header.Get("Idempotency-Replayed") != "true" {
		t.Fatal("replacement retry was not replayed")
	}
	errorCode(t, h.request("POST", "/credential-resolutions", map[string]any{"credential_value": f.ivanCard.Value}, "", 403), "CREDENTIAL_INACTIVE")
	h.request("POST", "/credential-resolutions", map[string]any{"credential_value": newCard.Value}, "", 200)

	// Naming an old, disabled card must not bypass the current card's slot.
	errorCode(t, h.request("POST", path, map[string]any{
		"value": "00006789", "replaces_credential_id": f.ivanCard.ID,
	}, uuid.NewString(), 409), "EMPLOYEE_ACTIVE_CREDENTIAL_EXISTS")
	if h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1", f.ivan.ID) != 2 {
		t.Fatal("replay or rejected replacement created another card")
	}

	// Loss can also be handled by disabling first, then issuing a new card.
	h.request("PATCH", path+"/"+newCard.ID, map[string]any{"is_active": false}, uuid.NewString(), 200)
	third := decode[credentialDTO](t, h.post(path, map[string]any{"value": "00006789"}))
	cards := decode[listDTO[credentialDTO]](t, h.get(path)).Items
	active := 0
	for _, card := range cards {
		if card.IsActive {
			active++
			if card.ID != third.ID {
				t.Fatal("wrong card remained active")
			}
		} else if card.DisabledAt == nil {
			t.Fatal("disabled card lost its historical timestamp")
		}
	}
	if len(cards) != 3 || active != 1 {
		t.Fatal("card history or single active card was not preserved")
	}
}

func TestIntegrationCredentialReplacementRollback(t *testing.T) {
	for _, failure := range []struct {
		name, table, event, raise, code string
		status                          int
	}{
		{"insert", "employee_credentials", "INSERT", "RAISE EXCEPTION 'injected card failure'", "INTERNAL_ERROR", 500},
		{"conflict", "employee_credentials", "INSERT", "RAISE EXCEPTION USING ERRCODE='23505', CONSTRAINT='credentials_active_value_uq'", "ACTIVE_CREDENTIAL_EXISTS", 409},
		{"response", "idempotency_requests", "UPDATE", "RAISE EXCEPTION 'injected response failure'", "INTERNAL_ERROR", 500},
	} {
		t.Run(failure.name, func(t *testing.T) {
			h := newHarness(t)
			f := h.fixture()
			path := "/employees/" + f.ivan.ID + "/credentials"
			before := h.get(path)
			_, err := h.owner.Exec(`CREATE FUNCTION force_card_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN ` + failure.raise + `; END $$; CREATE TRIGGER force_card_failure BEFORE ` + failure.event + ` ON ` + failure.table + ` FOR EACH ROW EXECUTE FUNCTION force_card_failure()`)
			if err != nil {
				t.Fatal(err)
			}
			key := uuid.NewString()
			body := map[string]any{"value": "00005678", "replaces_credential_id": f.ivanCard.ID}
			errorCode(t, h.request("POST", path, body, key, failure.status), failure.code)
			equalJSON(t, before, h.get(path))
			h.request("POST", "/credential-resolutions", map[string]any{"credential_value": f.ivanCard.Value}, "", 200)
			if failure.status == 500 && h.count("SELECT count(*) FROM idempotency_requests WHERE key=$1", key) != 0 {
				t.Fatal("failed command retained the request key")
			}
			if _, err := h.owner.Exec("DROP TRIGGER force_card_failure ON " + failure.table); err != nil {
				t.Fatal(err)
			}
			if failure.status == 409 {
				// Business conflicts remain replayable; a new attempt needs a new key.
				errorCode(t, h.request("POST", path, body, key, 409), failure.code)
				key = uuid.NewString()
			}
			h.request("POST", path, body, key, 201)
		})
	}
}

func TestIntegrationConcurrentSingleActiveCredential(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "create"
		if replace {
			name = "replace"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			employee := decode[employeeDTO](t, h.post("/employees", map[string]any{"display_name": "One card"}))
			path := "/employees/" + employee.ID + "/credentials"
			first, second := map[string]any{"value": "000001"}, map[string]any{"value": "000002"}
			if replace {
				old := decode[credentialDTO](t, h.post(path, map[string]any{"value": "000003"}))
				first["replaces_credential_id"], second["replaces_credential_id"] = old.ID, old.ID
			}
			results := h.parallel(commandJSON(t, "POST", path, uuid.NewString(), first), commandJSON(t, "POST", path, uuid.NewString(), second))
			oneWinner(t, results, "EMPLOYEE_ACTIVE_CREDENTIAL_EXISTS")
			if h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1 AND disabled_at IS NULL", employee.ID) != 1 {
				t.Fatal("concurrent requests created multiple active cards")
			}
			want := 1
			if replace {
				want = 2
			}
			if h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1", employee.ID) != want {
				t.Fatal("losing request left a card in history")
			}
		})
	}
}

func TestIntegrationSingleActiveCredentialDatabaseConstraint(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	_, err := h.app.Exec("INSERT INTO employee_credentials (id,employee_id,value) VALUES ($1,$2,$3)", uuid.NewString(), f.ivan.ID, "000001")
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.Code != "23505" || databaseError.ConstraintName != "credentials_one_active_per_employee_uq" {
		t.Fatalf("direct insert bypassed single active card constraint: %v", err)
	}
	if repository.ClassifyError(err).Code != "EMPLOYEE_ACTIVE_CREDENTIAL_EXISTS" {
		t.Fatal("database conflict did not map to the API error")
	}
	newID := uuid.NewString()
	if _, err := h.app.Exec("INSERT INTO employee_credentials (id,employee_id,value,disabled_at) VALUES ($1,$2,$3,clock_timestamp())", newID, f.ivan.ID, "000001"); err != nil {
		t.Fatal("historical cards must remain allowed:", err)
	}
	if _, err := h.app.Exec("UPDATE employee_credentials SET disabled_at=NULL WHERE id=$1", newID); err == nil {
		t.Fatal("direct update bypassed single active card constraint")
	}
}

func TestIntegrationConcurrentCredentialReplacementReplay(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	path := "/employees/" + f.ivan.ID + "/credentials"
	command := commandJSON(t, "POST", path, uuid.NewString(), map[string]any{
		"value": "00005678", "replaces_credential_id": f.ivanCard.ID,
	})
	results := h.parallel(command, command)
	h.check(results[0], 201)
	h.check(results[1], 201)
	equalJSON(t, results[0], results[1])
	if results[0].header.Get("Idempotency-Replayed") != "true" && results[1].header.Get("Idempotency-Replayed") != "true" {
		t.Fatal("concurrent replacement retry was not replayed")
	}
	if h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1", f.ivan.ID) != 2 ||
		h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1 AND disabled_at IS NULL", f.ivan.ID) != 1 {
		t.Fatal("concurrent retry replaced the card more than once")
	}
}
