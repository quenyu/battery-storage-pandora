package handler

import (
	"battery-storage-pandora/internal/model"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeRequestRejectsAmbiguousJSON(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `"text"`, `{"display_name":`,
		`{"display_name":"A","display_name":"B"}`,
		`{"display_name":null}`, `{"display_name":"A","unknown":1}`,
		`{"Display_Name":"A"}`, `{"DISPLAY_NAME":"A"}`,
		`{"display_name":42}`, `{"display_name":"A"} {}`,
		"{\"display_name\":\"\xff\"}",
	} {
		t.Run(body, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/employees", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			var input createEmployeeRequest
			err := decodeRequest(httptest.NewRecorder(), r, &input, "display_name personnel_number")
			if err == nil || classify(err).Status != http.StatusBadRequest {
				t.Fatalf("expected invalid request, got %v", err)
			}
		})
	}
}

func TestDecodeRequestBodyLimit(t *testing.T) {
	for _, size := range []int{64 << 10, (64 << 10) + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			body := `{"display_name":"` + strings.Repeat("A", size-len(`{"display_name":""}`)) + `"}`
			r := httptest.NewRequest(http.MethodPost, "/api/employees", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			var input createEmployeeRequest
			err := decodeRequest(httptest.NewRecorder(), r, &input, "display_name")
			if size == 64<<10 {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || classify(err).Status != http.StatusRequestEntityTooLarge {
				t.Fatalf("expected 413, got %v", err)
			}
		})
	}
}

func TestCanonicalRequestPreservesLegacyHashes(t *testing.T) {
	name, number, value := "Иван <Петров> &", "00004281", "Карта<&>"
	replacement := "00000000-0000-4000-8000-00000000000a"
	actor, version := "00001234", int64(9007199254740993)
	inactive := false
	for _, tc := range []struct {
		name, method, path, canonical string
		body                          any
	}{
		{"Unicode and omitted fields", "POST", "/api/employees", `{"display_name":"Иван \u003cПетров\u003e \u0026","personnel_number":"00004281"}`, &createEmployeeRequest{DisplayName: &name, PersonnelNumber: &number}},
		{"UUID in path", "POST", "/api/employees/00000000-0000-4000-8000-00000000000A/credentials", `{"replaces_credential_id":"00000000-0000-4000-8000-00000000000a","value":"Карта\u003c\u0026\u003e"}`, &createCredentialRequest{Value: &value, ReplacesCredentialID: &replacement}},
		{"exact int64", "POST", "/api/batteries/00000000-0000-4000-8000-000000000001/take", `{"actor_credential_value":"00001234","expected_version":9007199254740993}`, &takeBatteryRequest{ActorCredential: &actor, ExpectedVersion: &version}},
		{"supplied false", "PATCH", "/api/employees/00000000-0000-4000-8000-000000000001", `{"is_active":false}`, &patchEmployeeRequest{IsActive: &inactive}},
		{"empty patch", "PATCH", "/api/employees/00000000-0000-4000-8000-000000000001", `{}`, &patchEmployeeRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			request, err := commandRequest(w, r, tc.body)
			if err != nil {
				t.Fatal(err)
			}
			// These golden strings are independent of request encoding helpers.
			expected := sha256.Sum256([]byte(tc.method + "\n" + strings.ToLower(tc.path) + "\n" + tc.canonical))
			if request.Hash != fmt.Sprintf("%x", expected) {
				t.Fatalf("hash changed: got %s want %x", request.Hash, expected)
			}
		})
	}
}

func TestWriteCommandPreservesHeadersAndBody(t *testing.T) {
	for _, tc := range []struct{ name, path, body, location string }{
		{"employee", "/api/employees", `{"id":"employee"}`, "/api/employees/employee"},
		{"credential", "/api/employees/00000000-0000-4000-8000-00000000000A/credentials", `{"id":"credential"}`, "/api/employees/00000000-0000-4000-8000-00000000000a/credentials/credential"},
		{"operation", "/api/batteries", `{"battery":{"id":"battery"},"operation":{"id":"operation"}}`, "/api/operations/operation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, tc.path, nil)
			writeCommand(w, r, model.CommandResponse{Status: 201, Body: []byte(tc.body), Replayed: true}, nil)
			if w.Code != 201 || w.Header().Get("Location") != tc.location || w.Header().Get("Idempotency-Replayed") != "true" || !json.Valid(w.Body.Bytes()) {
				t.Fatalf("unexpected command response: %d %v %s", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestDecodeRequestPreservesFalseAndLargeInt64(t *testing.T) {
	for _, body := range []string{`{}`, `{"is_active":false}`} {
		r := httptest.NewRequest(http.MethodPatch, "/api/employees", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		var input patchEmployeeRequest
		if err := decodeRequest(httptest.NewRecorder(), r, &input, "is_active"); err != nil {
			t.Fatal(err)
		}
		if body == `{}` {
			if input.IsActive != nil {
				t.Fatal("missing is_active became a supplied value")
			}
		} else if input.IsActive == nil || *input.IsActive {
			t.Fatal("supplied false was lost")
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/batteries", strings.NewReader(`{"actor_credential_value":"00001234","expected_version":9007199254740993}`))
	r.Header.Set("Content-Type", "application/json")
	var input takeBatteryRequest
	if err := decodeRequest(httptest.NewRecorder(), r, &input, "actor_credential_value expected_version observed_source_location"); err != nil {
		t.Fatal(err)
	}
	if input.ExpectedVersion == nil || *input.ExpectedVersion != 9007199254740993 || input.ObservedSourceLocation != nil {
		t.Fatalf("precision or absent field changed: %+v", input)
	}
}
