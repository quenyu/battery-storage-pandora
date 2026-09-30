package swagger

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	assets "battery-storage-pandora"
)

func TestBundledUIResourceClosure(t *testing.T) {
	handler := Handler()
	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}
	redirect := request("/swagger")
	if redirect.Code != http.StatusPermanentRedirect || redirect.Header().Get("Location") != "/swagger/" {
		t.Fatalf("noncanonical redirect: %d %q", redirect.Code, redirect.Header().Get("Location"))
	}
	page := request("/swagger/")
	if page.Code != http.StatusOK || !strings.Contains(page.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("UI unavailable: %d %v", page.Code, page.Header())
	}
	if !strings.Contains(page.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("Swagger must restrict runtime network connections to its own origin")
	}
	references := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(references) < 3 {
		t.Fatal("Swagger UI must contain its stylesheet and scripts")
	}
	for _, reference := range references {
		path := reference[1]
		if !strings.HasPrefix(path, "./") {
			t.Fatalf("nonlocal runtime resource: %q", path)
		}
		response := request("/swagger/" + strings.TrimPrefix(path, "./"))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("missing embedded resource %s: %d", path, response.Code)
		}
	}
	config := request("/swagger/swagger-initializer.js").Body.String()
	for _, required := range []string{`url: "/openapi.yaml"`, `validatorUrl: null`, `queryConfigEnabled: false`} {
		if !strings.Contains(config, required) {
			t.Fatalf("missing offline configuration: %s", required)
		}
	}
	css := request("/swagger/swagger-ui.css").Body.String()
	if regexp.MustCompile(`(?i)(?:url\(\s*["']?(?:https?:)?//|@import)`).MatchString(css) {
		t.Fatal("Swagger stylesheet must not load external resources")
	}
	if request("/swagger/nonexistent.js").Code != http.StatusNotFound {
		t.Fatal("missing assets must return 404")
	}
}

func TestEmbeddedContractIsAuthoritativeSource(t *testing.T) {
	source, err := os.ReadFile("../docs/design/battery-storage-pandora/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, assets.OpenAPI) {
		t.Fatal("embedded contract differs from authoritative design source")
	}
	if !bytes.Contains(assets.OpenAPI, []byte("- url: /api/v1")) {
		t.Fatal("Try it out must use the same-origin API")
	}
}
