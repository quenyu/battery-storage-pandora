package handler_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestIntegrationReplaysLegacyRequestHashes(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*harness) (method, path, canonical, replay string, saved response)
	}{
		{"JSON order, escapes and Unicode", func(h *harness) (string, string, string, string, response) {
			saved := h.post("/employees", map[string]any{"display_name": "Иван <Петров> &", "personnel_number": "00004281"})
			return "POST", "/employees",
				`{"display_name":"Иван \u003cПетров\u003e \u0026","personnel_number":"00004281"}`,
				`{"personnel_number":"00004281","display_name":"\u0418ван <Петров> &"}`, saved
		}},
		{"replacement UUID normalization", func(h *harness) (string, string, string, string, response) {
			f := h.fixture()
			path := "/employees/" + f.ivan.ID + "/credentials"
			saved := h.post(path, map[string]any{"value": "Карта<&>", "replaces_credential_id": f.ivanCard.ID})
			return "POST", path,
				fmt.Sprintf(`{"replaces_credential_id":"%s","value":"Карта\u003c\u0026\u003e"}`, f.ivanCard.ID),
				fmt.Sprintf(`{"value":"\u041aарта<&>","replaces_credential_id":"%s"}`, strings.ToUpper(f.ivanCard.ID)), saved
		}},
		{"int64 above floating point precision", func(h *harness) (string, string, string, string, response) {
			h.fixture()
			battery := h.register("AKB-LEGACY", "1.1.1")
			path := "/batteries/" + battery.Battery.ID + "/take"
			saved := h.request("POST", path, map[string]any{"actor_credential_value": "00001234", "expected_version": int64(9007199254740993)}, uuid.NewString(), 409)
			return "POST", path,
				`{"actor_credential_value":"00001234","expected_version":9007199254740993}`,
				`{"expected_version":9007199254740993,"actor_credential_value":"00001234"}`, saved
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			method, path, canonical, body, saved := tt.setup(h)
			// Golden strings reproduce the old sorted map encoding. They deliberately
			// do not call the current request decoder or canonicalization helper.
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(method+"\n/api"+path+"\n"+canonical)))
			key := uuid.NewString()
			if _, err := h.owner.Exec(`INSERT INTO idempotency_requests
				(scope,key,request_hash,http_status,response_body) VALUES ('pandora',$1,$2,$3,$4)`, key, hash, saved.status, string(saved.body)); err != nil {
				t.Fatal(err)
			}
			before := requestStateSnapshot(t, h)
			replayed := h.raw(method, path, body, key, "application/json", saved.status)
			equalJSON(t, saved, replayed)
			if replayed.header.Get("Idempotency-Replayed") != "true" {
				t.Fatal("legacy request was executed instead of replayed")
			}
			if after := requestStateSnapshot(t, h); after != before {
				t.Fatal("replay changed employees, credentials, batteries, history or saved requests")
			}
		})
	}
}

func requestStateSnapshot(t *testing.T, h *harness) string {
	t.Helper()
	var snapshot string
	err := h.owner.QueryRow(`SELECT jsonb_build_object(
		'employees', (SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),'[]') FROM employees e),
		'credentials', (SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY id),'[]') FROM employee_credentials c),
		'batteries', (SELECT coalesce(jsonb_agg(to_jsonb(b) ORDER BY id),'[]') FROM batteries b),
		'operations', (SELECT coalesce(jsonb_agg(to_jsonb(o) ORDER BY id),'[]') FROM battery_operations o),
		'requests', (SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY scope,key),'[]') FROM idempotency_requests r)
	)::text`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestIntegrationTypedCommandValidation(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	battery := h.register("AKB-VALIDATION", "1.1.1")
	employeePath := "/employees/" + f.ivan.ID
	batteryPath := "/batteries/" + battery.Battery.ID
	tests := []struct {
		name, method, path string
		body               map[string]any
		required           []string
	}{
		{"create employee", "POST", "/employees", map[string]any{"display_name": "Employee", "personnel_number": "000042"}, []string{"display_name"}},
		{"patch employee", "PATCH", employeePath, map[string]any{"display_name": "Employee", "is_active": false}, nil},
		{"create credential", "POST", employeePath + "/credentials", map[string]any{"value": "NEW-CARD", "replaces_credential_id": f.ivanCard.ID}, []string{"value"}},
		{"disable credential", "PATCH", employeePath + "/credentials/" + f.ivanCard.ID, map[string]any{"is_active": false}, []string{"is_active"}},
		{"STORE", "POST", "/batteries", map[string]any{"inventory_code": "NEW-AKB", "serial_number": "SN-1", "actor_credential_value": "00001234", "destination_location": "2.1.1"}, []string{"inventory_code", "actor_credential_value", "destination_location"}},
		{"TAKE", "POST", batteryPath + "/take", map[string]any{"actor_credential_value": "00001234", "expected_version": int64(1), "observed_source_location": "1.1.1"}, []string{"actor_credential_value"}},
		{"RETURN", "POST", batteryPath + "/return", map[string]any{"actor_credential_value": "00001234", "destination_location": "2.1.1", "expected_version": int64(1)}, []string{"actor_credential_value", "destination_location"}},
		{"MOVE", "POST", batteryPath + "/move", map[string]any{"actor_credential_value": "00001234", "destination_location": "2.1.1", "expected_version": int64(1), "observed_source_location": "1.1.1"}, []string{"actor_credential_value", "destination_location"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := func(name, body string, status int) {
				t.Run(name, func(t *testing.T) {
					key := uuid.NewString()
					res := h.raw(tt.method, tt.path, body, key, "application/json", status)
					code := "INVALID_REQUEST"
					if status == 422 {
						code = "VALIDATION_FAILED"
					}
					errorCode(t, res, code)
					if h.count("SELECT count(*) FROM idempotency_requests WHERE key=$1", key) != 0 {
						t.Fatal("invalid JSON input persisted an idempotency key")
					}
				})
			}
			for _, field := range tt.required {
				check("missing "+field, changedRequestJSON(t, tt.body, field, nil, true), 400)
			}
			for field, value := range tt.body {
				check("null "+field, changedRequestJSON(t, tt.body, field, nil, false), 400)
				encoded, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				validJSON := changedRequestJSON(t, tt.body, "", nil, true)
				check("duplicate "+field, strings.TrimSuffix(validJSON, "}")+`,"`+field+`":`+string(encoded)+`}`, 400)
				switch value.(type) {
				case string:
					check("empty "+field, changedRequestJSON(t, tt.body, field, "", false), 422)
					check("wrong type "+field, changedRequestJSON(t, tt.body, field, 42, false), 400)
					if strings.HasSuffix(field, "_location") {
						for _, location := range []string{"01.1.1", "1.0.1", "1.1", " 1.1.1"} {
							check("invalid "+field+" "+location, changedRequestJSON(t, tt.body, field, location, false), 422)
						}
					}
					if field == "replaces_credential_id" {
						check("invalid UUID", changedRequestJSON(t, tt.body, field, "invalid-id", false), 400)
					}
				case bool:
					check("wrong type "+field, changedRequestJSON(t, tt.body, field, "false", false), 400)
				case int64:
					check("string version", changedRequestJSON(t, tt.body, field, "1", false), 400)
					check("fractional version", changedRequestJSON(t, tt.body, field, 1.5, false), 400)
					check("overflow version", changedRequestJSON(t, tt.body, field, json.Number("9223372036854775808"), false), 400)
					check("zero version", changedRequestJSON(t, tt.body, field, 0, false), 422)
				}
			}
			validJSON := changedRequestJSON(t, tt.body, "", nil, true)
			check("unknown field", strings.TrimSuffix(validJSON, "}")+`,"unknown":1}`, 400)
			check("trailing JSON", validJSON+" {}", 400)
		})
	}
	for _, tt := range []struct{ path, extra string }{
		{batteryPath + "/take", `"destination_location":"2.1.1"`},
		{batteryPath + "/return", `"destination_location":"2.1.1","observed_source_location":"1.1.1"`},
		{batteryPath + "/move", `"destination_location":"2.1.1","inventory_code":"NEW-AKB"`},
	} {
		key := uuid.NewString()
		errorCode(t, h.raw("POST", tt.path, `{"actor_credential_value":"00001234",`+tt.extra+`}`, key, "application/json", 400), "INVALID_REQUEST")
		if h.count("SELECT count(*) FROM idempotency_requests WHERE key=$1", key) != 0 {
			t.Fatal("command accepted a field belonging to another command")
		}
	}
	// false is a supplied value, including for the mandatory card field.
	card := h.request("PATCH", employeePath+"/credentials/"+f.ivanCard.ID, map[string]any{"is_active": false}, uuid.NewString(), 200)
	if decode[credentialDTO](t, card).IsActive {
		t.Fatal("mandatory false was lost")
	}
	employee := h.request("PATCH", employeePath, map[string]any{"is_active": false}, uuid.NewString(), 200)
	if decode[employeeDTO](t, employee).IsActive {
		t.Fatal("optional false was lost")
	}
	// The existing service-level rule differs from malformed input: an empty
	// employee patch is a saved 422 business result, replayed on the same key.
	key := uuid.NewString()
	emptyPatch := h.raw("PATCH", employeePath, `{}`, key, "application/json", 422)
	errorCode(t, emptyPatch, "VALIDATION_FAILED")
	equalJSON(t, emptyPatch, h.raw("PATCH", employeePath, `{}`, key, "application/json", 422))
}

func changedRequestJSON(t *testing.T, original map[string]any, field string, value any, remove bool) string {
	t.Helper()
	body := make(map[string]any, len(original))
	for key, item := range original {
		body[key] = item
	}
	if remove {
		delete(body, field)
	} else {
		body[field] = value
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
