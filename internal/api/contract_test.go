package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	assets "battery-storage-pandora"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
)

type contractChecker struct {
	doc    *openapi3.T
	router routers.Router
	seen   map[string]bool
}

func newContractChecker(t *testing.T) *contractChecker {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(assets.OpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("OpenAPI invalid: %v", err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return &contractChecker{doc: doc, router: router, seen: map[string]bool{}}
}
func (c *contractChecker) validate(t *testing.T, res response) {
	t.Helper()
	// The authoritative server is same-origin /api. kin-openapi's legacy
	// router matches relative server URLs against relative request URLs.
	request := res.request.Clone(context.Background())
	requestURL := *request.URL
	requestURL.Scheme, requestURL.Host = "", ""
	request.URL = &requestURL
	route, params, err := c.router.FindRoute(request)
	if err != nil {
		t.Fatalf("request not present in OpenAPI: %s %s: %v", res.request.Method, res.request.URL, err)
	}
	input := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, PathParams: params, Route: route}, Status: res.status, Header: res.header, Options: &openapi3filter.Options{IncludeResponseStatus: true}}
	input.SetBodyBytes(res.body)
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		t.Fatalf("OpenAPI response violation for %s %s [%d]: %v\n%s", res.request.Method, res.request.URL.Path, res.status, err, res.body)
	}
	if res.status >= 200 && res.status < 300 {
		c.seen[route.Operation.OperationID] = true
	}
}
func (c *contractChecker) assertAllOperations(t *testing.T) {
	t.Helper()
	count := 0
	for path, item := range c.doc.Paths.Map() {
		for method, op := range item.Operations() {
			count++
			if !c.seen[op.OperationID] {
				t.Errorf("no successful contract-validated response captured for %s %s (%s)", method, path, op.OperationID)
			}
		}
	}
	if count != 19 {
		t.Errorf("contract contains %d operations, expected agreed 19", count)
	}
}

func TestOpenAPIContractAndExamples(t *testing.T) {
	c := newContractChecker(t)
	source, err := os.ReadFile("../../docs/design/battery-storage-discovery/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, assets.OpenAPI) {
		t.Fatal("embedded OpenAPI differs from the editable source")
	}
	if len(c.doc.Servers) != 1 || c.doc.Servers[0].URL != "/api" {
		t.Fatal("Swagger requests must use same-origin /api")
	}
	if c.doc.Info.Title != "Pandora — учёт АКБ" || c.doc.Info.Description != "" {
		t.Fatal("Swagger must use the agreed simple title without an info description")
	}
	if len(c.doc.Security) != 0 || len(c.doc.Components.SecuritySchemes) != 0 {
		t.Fatal("public API contract must not declare authentication")
	}
	if len(c.doc.Paths.Map()) != 15 || len(c.doc.Components.Schemas) != 20 {
		t.Fatalf("unexpected contract shape: %d paths %d schemas", len(c.doc.Paths.Map()), len(c.doc.Components.Schemas))
	}
	seen := map[string]bool{}
	operations, commands, examples := 0, 0, 0
	checkContent := func(label string, content openapi3.Content) {
		if content["application/json"] == nil || content["application/json"].Schema == nil {
			t.Errorf("%s must declare its application/json schema", label)
		}
		for _, media := range content {
			if media.Example == nil && len(media.Examples) == 0 {
				t.Errorf("%s has no request/response example", label)
			}
			check := func(value any) {
				if value == nil {
					return
				}
				examples++
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var normalized any
				if err := json.Unmarshal(data, &normalized); err != nil {
					t.Fatal(err)
				}
				if media.Schema == nil {
					t.Errorf("%s has example without schema", label)
					return
				}
				if err := media.Schema.Value.VisitJSON(normalized); err != nil {
					t.Errorf("%s invalid example: %v", label, err)
				}
			}
			check(media.Example)
			for _, example := range media.Examples {
				check(example.Value.Value)
			}
		}
	}
	for path, item := range c.doc.Paths.Map() {
		for method, op := range item.Operations() {
			operations++
			if op.OperationID == "" || seen[op.OperationID] {
				t.Errorf("missing/duplicate operationId at %s %s", method, path)
			}
			seen[op.OperationID] = true
			if op.Security != nil && len(*op.Security) != 0 {
				t.Errorf("%s %s unexpectedly requires authentication", method, path)
			}
			if op.Responses.Value("401") != nil {
				t.Errorf("%s %s documents a removed authentication response", method, path)
			}
			if (method == "POST" || method == "PATCH") && op.OperationID != "resolveCredential" {
				commands++
				hasKey := false
				for _, p := range op.Parameters {
					if p.Value.Name == "Idempotency-Key" && p.Value.In == "header" && p.Value.Required {
						hasKey = true
					}
				}
				if !hasKey {
					t.Errorf("%s %s missing required manual Idempotency-Key", method, path)
				}
			}
			if op.RequestBody != nil {
				checkContent(method+" "+path+" request", op.RequestBody.Value.Content)
			}
			for status, res := range op.Responses.Map() {
				checkContent(method+" "+path+" response "+status, res.Value.Content)
			}
		}
	}
	if operations != 19 || commands != 8 {
		t.Fatalf("got %d operations / %d commands; want 19 / 8", operations, commands)
	}
	t.Logf("OpenAPI valid: %d paths, %d operations, %d schemas, %d example occurrences checked", len(c.doc.Paths.Map()), operations, len(c.doc.Components.Schemas), examples)
}
