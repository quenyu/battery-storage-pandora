package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"battery-storage-pandora/internal/api"
	"battery-storage-pandora/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

type response struct {
	status  int
	body    []byte
	header  http.Header
	request *http.Request
}
type harness struct {
	t          *testing.T
	owner, app *sql.DB
	config     *pgx.ConnConfig
	server     *httptest.Server
	contract   *contractChecker
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for real PostgreSQL integration tests")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Never migrate, truncate, or drop objects in the local demonstration database.
	if cfg.Database != "pandora_test" && !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("TEST_DATABASE_URL database must end in _test")
	}
	admin := stdlib.OpenDB(*cfg)
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec("CREATE SCHEMA " + quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + quoted + " CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	cfg.RuntimeParams["search_path"] = schema
	owner := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { owner.Close() })
	if err := migrations.Up(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(context.Background(), owner); err != nil {
		t.Fatalf("second migration must be idempotent: %v", err)
	}
	appcfg := cfg.Copy()
	if appDSN := os.Getenv("TEST_APP_DATABASE_URL"); appDSN != "" {
		appcfg, err = pgx.ParseConfig(appDSN)
		if err != nil {
			t.Fatal(err)
		}
		if appcfg.Database != cfg.Database || appcfg.Host != cfg.Host || appcfg.Port != cfg.Port {
			t.Fatal("TEST_APP_DATABASE_URL must point to the same test database/server")
		}
		appcfg.RuntimeParams["search_path"] = schema
		role := pgx.Identifier{appcfg.User}.Sanitize()
		if _, err := owner.Exec("GRANT USAGE ON SCHEMA " + quoted + " TO " + role + "; GRANT SELECT, INSERT, UPDATE ON employees,employee_credentials,batteries,idempotency_requests TO " + role + "; GRANT SELECT,INSERT ON battery_operations TO " + role); err != nil {
			t.Fatal(err)
		}
	}
	app := stdlib.OpenDB(*appcfg)
	app.SetMaxOpenConns(16)
	t.Cleanup(func() { app.Close() })
	if err := app.Ping(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.New(app))
	t.Cleanup(srv.Close)
	return &harness{t: t, owner: owner, app: app, config: appcfg, server: srv, contract: newContractChecker(t)}
}

func perform(client *http.Client, base, method, path, body, key, contentType string) (response, error) {
	req, err := http.NewRequest(method, base+"/api"+path, strings.NewReader(body))
	if err != nil {
		return response{}, err
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	return response{res.StatusCode, data, res.Header, req}, err
}
func (h *harness) raw(method, path, body, key, contentType string, want int) response {
	h.t.Helper()
	res, err := perform(h.server.Client(), h.server.URL, method, path, body, key, contentType)
	if err != nil {
		h.t.Fatal(err)
	}
	h.check(res, want)
	return res
}
func (h *harness) check(res response, want int) {
	h.t.Helper()
	if res.status != want {
		h.t.Fatalf("%s %s: status %d want %d: %s", res.request.Method, res.request.URL.Path, res.status, want, res.body)
	}
	h.contract.validate(h.t, res)
}
func (h *harness) request(method, path string, body any, key string, want int) response {
	h.t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
	}
	return h.raw(method, path, string(data), key, "application/json", want)
}
func (h *harness) post(path string, body any) response {
	h.t.Helper()
	return h.request("POST", path, body, uuid.NewString(), 201)
}
func (h *harness) get(path string) response {
	h.t.Helper()
	return h.request("GET", path, nil, "", 200)
}
func decode[T any](t *testing.T, res response) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(res.body, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func equalJSON(t *testing.T, a, b response) {
	t.Helper()
	var x, y any
	if json.Unmarshal(a.body, &x) != nil || json.Unmarshal(b.body, &y) != nil {
		t.Fatal("invalid response JSON")
	}
	if a.status != b.status || !reflect.DeepEqual(x, y) {
		t.Fatalf("replayed response differs:\n%s\n%s", a.body, b.body)
	}
}
func errorCode(t *testing.T, res response, want string) {
	t.Helper()
	v := decode[map[string]any](t, res)
	if v["error"].(map[string]any)["code"] != want {
		t.Fatalf("error = %s want %s", res.body, want)
	}
}
func (h *harness) count(query string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.owner.QueryRow(query, args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}
func strptr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Test DTOs are independent of runtime structs. OpenAPI validates the complete
// response including additional fields, required nulls, formats and timestamps.
type employeeDTO struct {
	ID              string  `json:"id"`
	DisplayName     string  `json:"display_name"`
	PersonnelNumber *string `json:"personnel_number"`
	IsActive        bool    `json:"is_active"`
}
type credentialDTO struct {
	ID         string     `json:"id"`
	EmployeeID string     `json:"employee_id"`
	Value      string     `json:"value"`
	IsActive   bool       `json:"is_active"`
	DisabledAt *time.Time `json:"disabled_at"`
}
type batteryDTO struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`
	CurrentLocation *string   `json:"current_location"`
	CurrentHolder   *string   `json:"current_holder_employee_id"`
	Version         int64     `json:"version"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type operationDTO struct {
	ID                  string    `json:"id"`
	BatteryID           string    `json:"battery_id"`
	Type                string    `json:"type"`
	Actor               string    `json:"actor_employee_id"`
	CredentialID        string    `json:"credential_id"`
	BatteryVersion      int64     `json:"battery_version"`
	SourceLocation      *string   `json:"source_location"`
	DestinationLocation *string   `json:"destination_location"`
	SourceHolder        *string   `json:"source_holder_employee_id"`
	DestinationHolder   *string   `json:"destination_holder_employee_id"`
	OccurredAt          time.Time `json:"occurred_at"`
}
type commandDTO struct {
	Battery   batteryDTO   `json:"battery"`
	Operation operationDTO `json:"operation"`
}
type pageDTO[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
type fixture struct {
	ivan, olga         employeeDTO
	ivanCard, olgaCard credentialDTO
}

func (h *harness) fixture() fixture {
	h.t.Helper()
	f := fixture{}
	f.ivan = decode[employeeDTO](h.t, h.post("/employees", map[string]any{"display_name": "Иван Петров", "personnel_number": "00004281"}))
	f.olga = decode[employeeDTO](h.t, h.post("/employees", map[string]any{"display_name": "Ольга Иванова"}))
	f.ivanCard = decode[credentialDTO](h.t, h.post("/employees/"+f.ivan.ID+"/credentials", map[string]any{"value": "00001234"}))
	f.olgaCard = decode[credentialDTO](h.t, h.post("/employees/"+f.olga.ID+"/credentials", map[string]any{"value": "00009876"}))
	return f
}
func storeBody(code, location string) map[string]any {
	return map[string]any{"inventory_code": code, "actor_credential_value": "00001234", "destination_location": location}
}
func (h *harness) register(code, location string) commandDTO {
	h.t.Helper()
	return decode[commandDTO](h.t, h.post("/batteries", storeBody(code, location)))
}
func actionBody(card, destination string) map[string]any {
	body := map[string]any{"actor_credential_value": card}
	if destination != "" {
		body["destination_location"] = destination
	}
	return body
}
func (h *harness) action(id, action, card, destination string) response {
	h.t.Helper()
	return h.post("/batteries/"+id+"/"+action, actionBody(card, destination))
}
func (h *harness) battery(id string) batteryDTO {
	h.t.Helper()
	return decode[batteryDTO](h.t, h.get("/batteries/"+id))
}
func (h *harness) operations(path string) []operationDTO {
	h.t.Helper()
	return decode[pageDTO[operationDTO]](h.t, h.get(path)).Items
}

func TestIntegrationAll19RoutesAndStableIdentity(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	if strptr(f.ivan.PersonnelNumber) != "00004281" || f.ivanCard.Value != "00001234" {
		t.Fatal("leading zero identity lost")
	}
	resolved := h.request("POST", "/credential-resolutions", map[string]any{"credential_value": "00001234"}, "", 200)
	if decode[struct {
		Employee employeeDTO `json:"employee"`
	}](t, resolved).Employee.ID != f.ivan.ID {
		t.Fatal("wrong card resolution")
	}
	r := h.register("AKB-0001", "1.1.1")
	h.register("AKB-0002", "1.1.2")
	id, key := r.Battery.ID, uuid.NewString()
	body := actionBody("00001234", "")
	body["expected_version"], body["observed_source_location"] = 1, "1.1.1"
	take := h.request("POST", "/batteries/"+id+"/take", body, key, 201)
	taken := decode[commandDTO](t, take)
	if taken.Battery.Status != "ISSUED" || strptr(taken.Battery.CurrentHolder) != f.ivan.ID || taken.Battery.Version != 2 || strptr(taken.Operation.SourceLocation) != "1.1.1" || taken.Operation.CredentialID != f.ivanCard.ID {
		t.Fatal("bad TAKE projection/history")
	}
	custody := decode[pageDTO[batteryDTO]](t, h.get("/employees/"+f.ivan.ID+"/batteries"))
	if len(custody.Items) != 1 || custody.Items[0].ID != id {
		t.Fatal("wrong custody")
	}
	errorCode(t, h.request("POST", "/batteries/"+id+"/return", actionBody("00009876", "1.2.4"), uuid.NewString(), 403), "RETURN_NOT_ALLOWED")
	if h.battery(id).Version != 2 {
		t.Fatal("foreign RETURN changed holder")
	}
	errorCode(t, h.request("PATCH", "/employees/"+f.ivan.ID, map[string]any{"is_active": false}, uuid.NewString(), 409), "EMPLOYEE_HAS_CUSTODY")
	newCard := decode[credentialDTO](t, h.post("/employees/"+f.ivan.ID+"/credentials", map[string]any{"value": "00005678", "replaces_credential_id": f.ivanCard.ID}))
	if newCard.EmployeeID != f.ivan.ID || newCard.ID == f.ivanCard.ID {
		t.Fatal("replacement lost employee identity")
	}
	errorCode(t, h.request("POST", "/credential-resolutions", map[string]any{"credential_value": "00001234"}, "", 403), "CREDENTIAL_INACTIVE")
	ret := decode[commandDTO](t, h.action(id, "return", "00005678", "1.2.4"))
	if ret.Battery.Version != 3 || ret.Operation.Actor != f.ivan.ID || strptr(ret.Operation.SourceHolder) != f.ivan.ID || ret.Operation.CredentialID != newCard.ID {
		t.Fatal("same employee could not return with new card")
	}
	replay := h.request("POST", "/batteries/"+id+"/take", body, key, 201)
	equalJSON(t, take, replay)
	if replay.header.Get("Idempotency-Replayed") != "true" || h.battery(id).Version != 3 {
		t.Fatal("old TAKE replay changed current")
	}
	moved := decode[commandDTO](t, h.action(id, "move", "00005678", "2.1.2"))
	if strptr(moved.Operation.SourceLocation) != "1.2.4" || strptr(moved.Operation.DestinationLocation) != "2.1.2" || moved.Battery.Version != 4 || !moved.Operation.OccurredAt.Equal(moved.Battery.UpdatedAt) {
		t.Fatal("MOVE/time projection wrong")
	}
	h.get("/operations/" + moved.Operation.ID)
	before := h.get("/operations/" + taken.Operation.ID)
	if decode[operationDTO](t, before).CredentialID != f.ivanCard.ID {
		t.Fatal("old TAKE credential changed")
	}
	disabled := decode[credentialDTO](t, h.request("PATCH", "/employees/"+f.ivan.ID+"/credentials/"+newCard.ID, map[string]any{"is_active": false}, uuid.NewString(), 200))
	again := decode[credentialDTO](t, h.request("PATCH", "/employees/"+f.ivan.ID+"/credentials/"+newCard.ID, map[string]any{"is_active": false}, uuid.NewString(), 200))
	if disabled.DisabledAt == nil || again.DisabledAt == nil || !disabled.DisabledAt.Equal(*again.DisabledAt) {
		t.Fatal("repeat disable changed historical time")
	}
	equalJSON(t, before, h.get("/operations/"+taken.Operation.ID))
	h.request("PATCH", "/employees/"+f.ivan.ID, map[string]any{"display_name": "Иван Петров II", "is_active": false}, uuid.NewString(), 200)
	for _, path := range []string{"/employees", "/employees/" + f.ivan.ID, "/employees/" + f.ivan.ID + "/credentials", "/batteries", "/batteries/" + id, "/employees/" + f.ivan.ID + "/operations", "/operations"} {
		h.get(path)
	}
	history := h.operations("/batteries/" + id + "/operations")
	if len(history) != 4 {
		t.Fatalf("history length %d", len(history))
	}
	for i, typ := range []string{"MOVE", "RETURN", "TAKE", "STORE"} {
		if history[i].Type != typ || history[i].BatteryVersion != int64(4-i) {
			t.Fatal("history not ordered by version")
		}
	}
	at := url.QueryEscape(taken.Operation.OccurredAt.Format(time.RFC3339Nano))
	if len(h.operations("/operations?battery_id="+id+"&from="+at)) != 3 || len(h.operations("/operations?battery_id="+id+"&to="+at)) != 1 {
		t.Fatal("history time bounds must be inclusive from/exclusive to")
	}
	if len(h.operations("/operations?location=1.2.4")) != 2 {
		t.Fatal("history address filter must cover source and destination")
	}
	if len(h.operations("/operations?employee_id="+f.ivan.ID)) != 5 {
		t.Fatal("employee history duplicated/missed")
	}
	occupied := decode[pageDTO[batteryDTO]](t, h.get("/batteries?location=2.1.2"))
	if len(occupied.Items) != 1 || occupied.Items[0].ID != id {
		t.Fatal("exact address contents wrong")
	}
	if len(decode[pageDTO[batteryDTO]](t, h.get("/batteries?location=99.99.99")).Items) != 0 {
		t.Fatal("empty exact address should have no batteries")
	}
	if h.count("SELECT count(*) FROM battery_operations WHERE request_scope<>'pandora' OR device_code IS NOT NULL") != 0 {
		t.Fatal("public API operations must use one request scope and no device identity")
	}
	h.contract.assertAllOperations(t)
}

type concurrentCommand struct{ method, path, body, key string }

func commandJSON(t *testing.T, method, path, key string, body any) concurrentCommand {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return concurrentCommand{method, path, string(b), key}
}
func (h *harness) parallel(a, b concurrentCommand) [2]response {
	h.t.Helper()
	commands := []concurrentCommand{a, b}
	var servers [2]*httptest.Server
	var pids [2]int
	for i := range servers {
		db := stdlib.OpenDB(*h.config)
		db.SetMaxOpenConns(1)
		defer db.Close()
		if err := db.QueryRow("SELECT pg_backend_pid()").Scan(&pids[i]); err != nil {
			h.t.Fatal(err)
		}
		servers[i] = httptest.NewServer(api.New(db))
		defer servers[i].Close()
	}
	if pids[0] == pids[1] {
		h.t.Fatal("concurrency must use independent PostgreSQL sessions")
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var results [2]response
	var errs [2]error
	for i := range commands {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			c := commands[i]
			results[i], errs[i] = perform(servers[i].Client(), servers[i].URL, c.method, c.path, c.body, c.key, "application/json")
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			h.t.Fatal(err)
		}
		h.contract.validate(h.t, results[i])
	}
	return results
}
func oneWinner(t *testing.T, r [2]response, code string) int {
	t.Helper()
	winner := -1
	for i, res := range r {
		if res.status == 201 {
			if winner >= 0 {
				t.Fatal("both concurrent requests succeeded")
			}
			winner = i
		} else {
			if res.status != 409 {
				t.Fatalf("unexpected %d: %s", res.status, res.body)
			}
			errorCode(t, res, code)
		}
	}
	if winner < 0 {
		t.Fatal("neither concurrent request succeeded")
	}
	return winner
}
func TestIntegrationConcurrentTake(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	r := h.register("RACE", "1.1.1")
	path := "/batteries/" + r.Battery.ID + "/take"
	results := h.parallel(commandJSON(t, "POST", path, uuid.NewString(), actionBody("00001234", "")), commandJSON(t, "POST", path, uuid.NewString(), actionBody("00009876", "")))
	winner := oneWinner(t, results, "BATTERY_ALREADY_ISSUED")
	won := decode[commandDTO](t, results[winner])
	current := h.battery(r.Battery.ID)
	if current.Version != 2 || strptr(current.CurrentHolder) != strptr(won.Battery.CurrentHolder) || h.count("SELECT count(*) FROM battery_operations WHERE type='TAKE'") != 1 {
		t.Fatal("double TAKE")
	}
}
func TestIntegrationConcurrentMoveAndRegister(t *testing.T) {
	t.Run("move", func(t *testing.T) {
		h := newHarness(t)
		h.fixture()
		a := h.register("A", "3.1.1")
		b := h.register("B", "3.1.2")
		body := actionBody("00001234", "3.1.3")
		r := h.parallel(commandJSON(t, "POST", "/batteries/"+a.Battery.ID+"/move", uuid.NewString(), body), commandJSON(t, "POST", "/batteries/"+b.Battery.ID+"/move", uuid.NewString(), body))
		win := oneWinner(t, r, "LOCATION_OCCUPIED")
		initial := []commandDTO{a, b}
		loser := h.battery(initial[1-win].Battery.ID)
		if loser.Version != 1 || strptr(loser.CurrentLocation) != strptr(initial[1-win].Battery.CurrentLocation) || h.count("SELECT count(*) FROM battery_operations WHERE type='MOVE'") != 1 || h.count("SELECT count(*) FROM batteries WHERE current_location='3.1.3'") != 1 {
			t.Fatal("losing MOVE changed source/history or double occupied destination")
		}
	})
	for _, sameInventory := range []bool{false, true} {
		t.Run(fmt.Sprint("register-same-inventory-", sameInventory), func(t *testing.T) {
			h := newHarness(t)
			h.fixture()
			b := storeBody("B", "3.1.3")
			code := "LOCATION_OCCUPIED"
			if sameInventory {
				b = storeBody("A", "3.1.4")
				code = "INVENTORY_CODE_EXISTS"
			}
			r := h.parallel(commandJSON(t, "POST", "/batteries", uuid.NewString(), storeBody("A", "3.1.3")), commandJSON(t, "POST", "/batteries", uuid.NewString(), b))
			oneWinner(t, r, code)
			if h.count("SELECT count(*) FROM batteries") != 1 || h.count("SELECT count(*) FROM battery_operations") != 1 {
				t.Fatal("losing registration left partial state")
			}
		})
	}
}
func TestIntegrationConcurrentReplayAndKeyConflict(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	reg := h.register("REPLAY", "3.2.1")
	key := uuid.NewString()
	path := "/batteries/" + reg.Battery.ID + "/take"
	c := commandJSON(t, "POST", path, key, actionBody("00001234", ""))
	r := h.parallel(c, c)
	h.check(r[0], 201)
	h.check(r[1], 201)
	equalJSON(t, r[0], r[1])
	if h.count("SELECT count(*) FROM battery_operations WHERE type='TAKE'") != 1 || h.count("SELECT count(*) FROM idempotency_requests WHERE key=$1", key) != 1 || (r[0].header.Get("Idempotency-Replayed") == "true") == (r[1].header.Get("Idempotency-Replayed") == "true") {
		t.Fatal("concurrent replay applied more than once or missing replay header")
	}
	conflictKey := uuid.NewString()
	r = h.parallel(commandJSON(t, "POST", "/employees", conflictKey, map[string]any{"display_name": "A"}), commandJSON(t, "POST", "/employees", conflictKey, map[string]any{"display_name": "B"}))
	oneWinner(t, r, "IDEMPOTENCY_KEY_REUSED")
	errorCode(t, h.request("POST", path, actionBody("00009876", ""), key, 409), "IDEMPOTENCY_KEY_REUSED")
	errorCode(t, h.request("POST", "/employees", map[string]any{"display_name": "X"}, key, 409), "IDEMPOTENCY_KEY_REUSED")
}
func TestIntegrationCachedConflictAfterAddressFreed(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	reg := h.register("FIRST", "3.2.2")
	key := uuid.NewString()
	body := storeBody("SECOND", "3.2.2")
	conflict := h.request("POST", "/batteries", body, key, 409)
	errorCode(t, conflict, "LOCATION_OCCUPIED")
	if h.count("SELECT count(*) FROM batteries") != 1 || h.count("SELECT count(*) FROM battery_operations") != 1 || h.count("SELECT count(*) FROM idempotency_requests WHERE key=$1 AND http_status=409", key) != 1 {
		t.Fatal("failed command not rolled back and cached atomically")
	}
	h.action(reg.Battery.ID, "take", "00001234", "")
	replay := h.request("POST", "/batteries", body, key, 409)
	equalJSON(t, conflict, replay)
	if replay.header.Get("Idempotency-Replayed") != "true" || h.count("SELECT count(*) FROM batteries") != 1 {
		t.Fatal("cached conflict retried effect")
	}
	h.request("POST", "/batteries", body, uuid.NewString(), 201)
}
func TestIntegrationRollbackAtEventAndResult(t *testing.T) {
	for _, target := range []string{"battery_operations", "idempotency_requests"} {
		t.Run(target, func(t *testing.T) {
			h := newHarness(t)
			h.fixture()
			reg := h.register("ROLLBACK", "3.2.1")
			before := h.get("/batteries/" + reg.Battery.ID)
			key := uuid.NewString()
			event := "INSERT"
			if target == "idempotency_requests" {
				event = "UPDATE"
			}
			_, err := h.owner.Exec(`CREATE FUNCTION force_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected private SQL failure'; END $$; CREATE TRIGGER force_failure BEFORE ` + event + ` ON ` + target + ` FOR EACH ROW EXECUTE FUNCTION force_failure()`)
			if err != nil {
				t.Fatal(err)
			}
			res := h.request("POST", "/batteries/"+reg.Battery.ID+"/take", actionBody("00001234", ""), key, 500)
			errorCode(t, res, "INTERNAL_ERROR")
			if bytes.Contains(res.body, []byte("injected private")) {
				t.Fatal("internal SQL leaked")
			}
			equalJSON(t, before, h.get("/batteries/"+reg.Battery.ID))
			if h.count("SELECT count(*) FROM battery_operations") != 1 || h.count("SELECT count(*) FROM idempotency_requests WHERE key=$1", key) != 0 {
				t.Fatal("failure did not roll back operation/key")
			}
			if _, err := h.owner.Exec("DROP TRIGGER force_failure ON " + target); err != nil {
				t.Fatal(err)
			}
			h.request("POST", "/batteries/"+reg.Battery.ID+"/take", actionBody("00001234", ""), key, 201)
		})
	}
}
func TestIntegrationLostHTTPResponseAfterCommit(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	reg := h.register("DROPPED", "3.2.1")
	key := uuid.NewString()
	path := "/batteries/" + reg.Battery.ID + "/take"
	captured := make(chan response, 1)
	// Handler commits first, then a wrapper closes TCP without sending its body.
	drop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rr := httptest.NewRecorder()
		api.New(h.app).ServeHTTP(rr, r)
		captured <- response{rr.Code, bytes.Clone(rr.Body.Bytes()), rr.Header().Clone(), r}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer drop.Close()
	_, err := perform(drop.Client(), drop.URL, "POST", path, `{"actor_credential_value":"00001234"}`, key, "application/json")
	if err == nil {
		t.Fatal("client received intentionally dropped response")
	}
	original := <-captured
	h.check(original, 201)
	replay := h.request("POST", path, actionBody("00001234", ""), key, 201)
	equalJSON(t, original, replay)
	if h.count("SELECT count(*) FROM battery_operations") != 2 || h.battery(reg.Battery.ID).Version != 2 {
		t.Fatal("lost response caused duplicate operation")
	}
}

func TestIntegrationCredentialsInactiveAndUnresolved(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	errorCode(t, h.request("POST", "/credential-resolutions", map[string]any{"credential_value": "UNKNOWN"}, "", 404), "CREDENTIAL_NOT_FOUND")
	if h.count("SELECT count(*) FROM employees") != 2 {
		t.Fatal("unknown scan created employee")
	}
	h.post("/employees/"+f.ivan.ID+"/credentials", map[string]any{"value": "00000001"})
	if h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1 AND disabled_at IS NULL", f.ivan.ID) != 2 {
		t.Fatal("multiple active credentials unsupported")
	}
	errorCode(t, h.request("POST", "/employees/"+f.olga.ID+"/credentials", map[string]any{"value": "00001234"}, uuid.NewString(), 409), "ACTIVE_CREDENTIAL_EXISTS")
	h.request("PATCH", "/employees/"+f.ivan.ID+"/credentials/"+f.ivanCard.ID, map[string]any{"is_active": false}, uuid.NewString(), 200)
	errorCode(t, h.request("POST", "/employees/"+f.olga.ID+"/credentials", map[string]any{"value": "00001234"}, uuid.NewString(), 409), "CREDENTIAL_REUSE_UNCONFIRMED")
	h.request("PATCH", "/employees/"+f.ivan.ID, map[string]any{"is_active": false}, uuid.NewString(), 200)
	errorCode(t, h.request("POST", "/credential-resolutions", map[string]any{"credential_value": "00000001"}, "", 403), "EMPLOYEE_INACTIVE")
	body := storeBody("INACTIVE", "4.1.1")
	body["actor_credential_value"] = "00000001"
	errorCode(t, h.request("POST", "/batteries", body, uuid.NewString(), 403), "EMPLOYEE_INACTIVE")
	if h.count("SELECT count(*) FROM batteries") != 0 || h.count("SELECT count(*) FROM battery_operations") != 0 {
		t.Fatal("inactive actor performed STORE")
	}
}
func TestIntegrationInvalidTransitionsAndObservations(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	reg := h.register("TRANSITIONS", "4.1.1")
	id := reg.Battery.ID
	path := "/batteries/" + id
	for _, tc := range []struct {
		action string
		body   map[string]any
		code   string
	}{
		{"return", actionBody("00001234", "4.1.2"), "INVALID_BATTERY_STATE"},
		{"move", actionBody("00001234", "4.1.1"), "SAME_LOCATION"},
		{"take", map[string]any{"actor_credential_value": "00001234", "expected_version": 2}, "STATE_VERSION_MISMATCH"},
		{"move", map[string]any{"actor_credential_value": "00001234", "destination_location": "4.1.2", "observed_source_location": "9.9.9"}, "SOURCE_MISMATCH"},
	} {
		errorCode(t, h.request("POST", path+"/"+tc.action, tc.body, uuid.NewString(), 409), tc.code)
	}
	if h.battery(id).Version != 1 || h.count("SELECT count(*) FROM battery_operations") != 1 {
		t.Fatal("rejected observations changed state/history")
	}
	h.action(id, "take", "00001234", "")
	errorCode(t, h.request("POST", path+"/take", actionBody("00001234", ""), uuid.NewString(), 409), "BATTERY_ALREADY_ISSUED")
	errorCode(t, h.request("POST", path+"/move", actionBody("00001234", "4.1.2"), uuid.NewString(), 409), "INVALID_BATTERY_STATE")
	if h.count("SELECT count(*) FROM battery_operations") != 2 {
		t.Fatal("invalid transition appended history")
	}
}
func TestIntegrationValidationAndCursor(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	reg := h.register("VALIDATION", "4.1.1")
	before := h.count("SELECT count(*) FROM idempotency_requests")
	for _, tc := range []struct {
		method, path, body, key, content string
		status                           int
		code                             string
	}{
		{"POST", "/employees", `{"display_name":`, uuid.NewString(), "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{"display_name":"A"} {}`, uuid.NewString(), "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{"display_name":"A"}`, "", "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{"display_name":"A"}`, "not-uuid", "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{"display_name":"A"}`, uuid.NewString(), "text/plain", 415, "UNSUPPORTED_MEDIA_TYPE"},
		{"POST", "/employees", strings.Repeat(" ", 65537), uuid.NewString(), "application/json", 413, "PAYLOAD_TOO_LARGE"},
		{"POST", "/employees", `{"display_name":null}`, uuid.NewString(), "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{"display_name":"A","unknown":1}`, uuid.NewString(), "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{"display_name":"A","display_name":"B"}`, uuid.NewString(), "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{}`, uuid.NewString(), "application/json", 400, "INVALID_REQUEST"},
		{"POST", "/employees", `{"display_name":123}`, uuid.NewString(), "application/json", 400, "INVALID_REQUEST"},
		{"GET", "/employees?limit=0", "", "", "", 400, "INVALID_REQUEST"},
		{"GET", "/employees?limit=201", "", "", "", 400, "INVALID_REQUEST"},
		{"GET", "/employees?is_active=maybe", "", "", "", 400, "INVALID_REQUEST"},
		{"GET", "/batteries?status=LOST", "", "", "", 422, "VALIDATION_FAILED"},
		{"GET", "/batteries?holder_employee_id=bad", "", "", "", 400, "INVALID_REQUEST"},
		{"GET", "/operations?type=loss", "", "", "", 422, "VALIDATION_FAILED"},
		{"GET", "/operations?from=bad", "", "", "", 400, "INVALID_REQUEST"},
		{"GET", "/operations?from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", "", "", "", 422, "VALIDATION_FAILED"},
		{"GET", "/employees?cursor=bad", "", "", "", 400, "INVALID_REQUEST"},
	} {
		t.Run(fmt.Sprintf("%s %s %s", tc.method, tc.path, tc.body), func(t *testing.T) {
			errorCode(t, h.raw(tc.method, tc.path, tc.body, tc.key, tc.content, tc.status), tc.code)
		})
	}
	for _, location := range []string{"", " 4.1.2", "4.1.2 ", "01.1.1", "1.0.1", "1.1.01", "1.1", "x.y.z", "1.1.1\n"} {
		t.Run("address-"+location, func(t *testing.T) {
			errorCode(t, h.request("POST", "/batteries", storeBody("INVALID", location), uuid.NewString(), 422), "VALIDATION_FAILED")
		})
	}
	// Validation happens before idempotency reservation, including invalid addresses.
	if h.count("SELECT count(*) FROM idempotency_requests") != before || h.battery(reg.Battery.ID).Version != 1 {
		t.Fatal("validation persisted key or changed battery")
	}
	page := decode[pageDTO[employeeDTO]](t, h.get("/employees?limit=1"))
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("missing cursor")
	}
	next := decode[pageDTO[employeeDTO]](t, h.get("/employees?limit=1&cursor="+url.QueryEscape(*page.NextCursor)))
	if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatal("cursor repeated/skipped employee")
	}
	errorCode(t, h.request("GET", "/employees?limit=1&is_active=true&cursor="+url.QueryEscape(*page.NextCursor), nil, "", 400), "INVALID_REQUEST")
	errorCode(t, h.request("GET", "/batteries?cursor="+url.QueryEscape(*page.NextCursor), nil, "", 400), "INVALID_REQUEST")
}

func TestIntegrationPublicAPIAndIgnoredAuthorizationHeader(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	list := h.get("/employees/" + f.ivan.ID + "/credentials")
	if list.request.Header.Get("Authorization") != "" {
		t.Fatal("public API test unexpectedly sent an Authorization header")
	}
	key := uuid.NewString()
	body := `{"display_name":"Открытый API"}`
	original := h.raw("POST", "/employees", body, key, "application/json", 201)
	for _, header := range []string{"Bearer ignored-value", "Basic ignored-value", "malformed"} {
		req, err := http.NewRequest("POST", h.server.URL+"/api/employees", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", header)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		res, err := h.server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		replay := response{res.StatusCode, data, res.Header, req}
		h.check(replay, 201)
		equalJSON(t, original, replay)
		if replay.header.Get("Idempotency-Replayed") != "true" {
			t.Fatal("Authorization header changed public request scope")
		}
	}
	if h.count("SELECT count(*) FROM idempotency_requests WHERE key=$1 AND scope='pandora'", key) != 1 || h.count("SELECT count(*) FROM employees") != 3 {
		t.Fatal("public requests did not share the fixed request scope")
	}
}

func TestIntegrationDatabaseImmutableGuardsAndLeastPrivilege(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	reg := h.register("GUARDS", "4.1.1")
	for _, tc := range []struct {
		sql, code string
		args      []any
	}{
		{"UPDATE battery_operations SET source_location=source_location WHERE id=$1", "55000", []any{reg.Operation.ID}},
		{"DELETE FROM battery_operations WHERE id=$1", "55000", []any{reg.Operation.ID}},
		{"TRUNCATE battery_operations", "55000", nil},
		{"UPDATE employee_credentials SET value='OTHER' WHERE id=$1", "55000", []any{f.ivanCard.ID}},
		{"UPDATE batteries SET inventory_code='OTHER' WHERE id=$1", "55000", []any{reg.Battery.ID}},
		{"UPDATE idempotency_requests SET response_body='{}' WHERE http_status=201", "55000", nil},
		{"DELETE FROM idempotency_requests WHERE http_status=201", "55000", nil},
		{"UPDATE batteries SET current_location='4.1.2' WHERE id=$1", "23514", []any{reg.Battery.ID}},
		{"INSERT INTO idempotency_requests(scope,key,request_hash) VALUES('incomplete',$1,repeat('a',64))", "23514", []any{uuid.NewString()}},
	} {
		_, err := h.owner.Exec(tc.sql, tc.args...)
		var p *pgconn.PgError
		if !errors.As(err, &p) || p.Code != tc.code {
			t.Fatalf("guard %s: %v want SQLSTATE %s", tc.sql, err, tc.code)
		}
	}
	if os.Getenv("TEST_APP_DATABASE_URL") == "" {
		t.Log("TEST_APP_DATABASE_URL absent: restricted-role privileges not checked")
		return
	}
	for _, query := range []string{"UPDATE battery_operations SET device_code=device_code", "DELETE FROM battery_operations", "TRUNCATE battery_operations", "DELETE FROM batteries", "SELECT * FROM schema_migrations", "SELECT * FROM legacy_operations", "SELECT * FROM legacy_cells"} {
		_, err := h.app.Exec(query)
		var p *pgconn.PgError
		if !errors.As(err, &p) || p.Code != "42501" {
			t.Fatalf("runtime unexpectedly permitted %q: %v", query, err)
		}
	}
}

func TestIntegrationHistoryCursorUsesVersionAndStableBoundary(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	reg := h.register("PAGING", "6.1.1")
	h.action(reg.Battery.ID, "take", "00001234", "")
	h.action(reg.Battery.ID, "return", "00001234", "6.1.2")
	path := "/batteries/" + reg.Battery.ID + "/operations?limit=1"
	first := decode[pageDTO[operationDTO]](t, h.get(path))
	if len(first.Items) != 1 || first.Items[0].BatteryVersion != 3 || first.NextCursor == nil {
		t.Fatal("wrong initial battery history page")
	}
	// Higher versions created while paging are excluded from this continuation.
	h.action(reg.Battery.ID, "move", "00001234", "6.1.3")
	seen := map[string]bool{first.Items[0].ID: true}
	cursor := first.NextCursor
	for wantVersion := int64(2); wantVersion > 0; wantVersion-- {
		if cursor == nil {
			t.Fatal("premature history cursor end")
		}
		page := decode[pageDTO[operationDTO]](t, h.get(path+"&cursor="+url.QueryEscape(*cursor)))
		if len(page.Items) != 1 || page.Items[0].BatteryVersion != wantVersion || seen[page.Items[0].ID] {
			t.Fatal("history continuation duplicated, skipped, or included new version")
		}
		seen[page.Items[0].ID] = true
		cursor = page.NextCursor
	}
	if cursor != nil {
		t.Fatal("nonempty continuation after last battery history entry")
	}
	from := url.QueryEscape(reg.Operation.OccurredAt.Format(time.RFC3339Nano))
	allPath := "/operations?battery_id=" + reg.Battery.ID + "&from=" + from + "&limit=1"
	seen = map[string]bool{}
	for pageNumber := 0; pageNumber < 6; pageNumber++ {
		page := decode[pageDTO[operationDTO]](t, h.get(allPath))
		for _, op := range page.Items {
			if seen[op.ID] {
				t.Fatal("time cursor duplicate")
			}
			seen[op.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		allPath = "/operations?battery_id=" + reg.Battery.ID + "&from=" + from + "&limit=1&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	if len(seen) != 4 {
		t.Fatalf("time cursor lost rows: %d", len(seen))
	}
}

func TestIntegrationInventoryCodeLookupPreservesLiteralIdentity(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	code := " B-LITERAL "
	reg := h.register(code, "7.1.1")
	page := decode[pageDTO[batteryDTO]](t, h.get("/batteries?inventory_code="+url.QueryEscape(code)))
	if len(page.Items) != 1 || page.Items[0].ID != reg.Battery.ID {
		t.Fatal("exact inventory lookup changed the registered identity")
	}
	if len(decode[pageDTO[batteryDTO]](t, h.get("/batteries?inventory_code=B-LITERAL")).Items) != 0 {
		t.Fatal("inventory lookup silently normalized different codes")
	}
}
