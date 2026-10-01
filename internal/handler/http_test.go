package handler_test

import (
	"battery-storage-pandora/internal/handler"
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/swagger"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestHTTPBundledDocumentation(t *testing.T) {
	server := handler.New(nil)
	for _, path := range []string{"/openapi.yaml", "/swagger/", "/swagger/swagger-initializer.js", "/swagger/swagger-ui.css", "/swagger/swagger-ui-bundle.js"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			server.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK || w.Body.Len() == 0 {
				t.Fatalf("documentation unavailable: status=%d body bytes=%d", w.Code, w.Body.Len())
			}
			if !model.ValidUUID(w.Header().Get("X-Request-ID")) {
				t.Fatal("missing request ID")
			}
			if path == "/openapi.yaml" && !bytes.Equal(w.Body.Bytes(), swagger.OpenAPI) {
				t.Fatal("HTTP response differs from embedded OpenAPI")
			}
		})
	}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger", nil))
	if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != "/swagger/" {
		t.Fatalf("unexpected redirect: %d %q", w.Code, w.Header().Get("Location"))
	}
}

func TestHTTPUnknownRouteReturnsJSON(t *testing.T) {
	w := httptest.NewRecorder()
	handler.New(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if w.Code != http.StatusNotFound || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected response: status=%d headers=%v", w.Code, w.Header())
	}
	var body model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "RESOURCE_NOT_FOUND" || body.Error.RequestID != w.Header().Get("X-Request-ID") || !model.ValidUUID(body.Error.RequestID) {
		t.Fatalf("unexpected error: %+v", body)
	}
}

func TestHTTPAllContractRoutesAreRegistered(t *testing.T) {
	contract := newContractChecker(t)
	server := handler.New(nil)
	parameters := regexp.MustCompile(`\{[^}]+\}`)
	for path, item := range contract.doc.Paths.Map() {
		for method := range item.Operations() {
			t.Run(method+" "+path, func(t *testing.T) {
				// Invalid input must reach route validation without accessing the database.
				actual := parameters.ReplaceAllString(path, "00000000-0000-4000-8000-000000000001")
				r := httptest.NewRequest(method, "/api"+actual+"?unknown=1", nil)
				w := httptest.NewRecorder()
				server.ServeHTTP(w, r)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("route did not reach validation: %d %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestHTTPInvalidCommandsDoNotAccessDatabase(t *testing.T) {
	server := handler.New(nil)
	employee := "/api/employees/00000000-0000-4000-8000-000000000001"
	battery := "/api/batteries/00000000-0000-4000-8000-000000000001"
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/employees", `{}`, 400},
		{"PATCH", employee, `{"display_name":" "}`, 422},
		{"POST", employee + "/credentials", `{}`, 400},
		{"POST", employee + "/credentials", `{"value":"CARD","replaces_credential_id":""}`, 422},
		{"POST", employee + "/credentials", `{"value":"CARD","replaces_credential_id":"bad"}`, 400},
		{"PATCH", employee + "/credentials/00000000-0000-4000-8000-000000000002", `{}`, 400},
		{"POST", "/api/credential-resolutions", `{}`, 400},
		{"POST", "/api/batteries", `{}`, 400},
		{"POST", "/api/batteries", `{"inventory_code":"B","actor_credential_value":"CARD","destination_location":"01.1.1"}`, 422},
		{"POST", battery + "/take", `{}`, 400},
		{"POST", battery + "/take", `{"actor_credential_value":"CARD","expected_version":0}`, 422},
		{"POST", battery + "/take", `{"actor_credential_value":"CARD","destination_location":"1.1.1"}`, 400},
		{"POST", battery + "/return", `{"actor_credential_value":"CARD"}`, 400},
		{"POST", battery + "/move", `{"actor_credential_value":"CARD"}`, 400},
	} {
		t.Run(tc.method+" "+tc.path+" "+tc.body, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", "00000000-0000-4000-8000-000000000003")
			w := httptest.NewRecorder()
			server.ServeHTTP(w, r)
			if w.Code != tc.status || !json.Valid(w.Body.Bytes()) {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
