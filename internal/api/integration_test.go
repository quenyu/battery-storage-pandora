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
		if _, err := owner.Exec("GRANT USAGE ON SCHEMA " + quoted + " TO " + role + "; GRANT SELECT, INSERT, UPDATE ON employees,employee_credentials,cabinets,shelves,cells,batteries,idempotency_records TO " + role + "; GRANT SELECT,INSERT ON operations TO " + role); err != nil {
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
	req, err := http.NewRequest(method, base+"/api/v1"+path, strings.NewReader(body))
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
	if v["code"] != want {
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

type fixture struct {
	ivan, olga api.Employee
	cabinets   []api.Cabinet
	shelves    []api.Shelf
	cells      []api.Cell
}

func (h *harness) fixture() fixture {
	h.t.Helper()
	f := fixture{}
	f.ivan = decode[api.Employee](h.t, h.post("/employees", map[string]any{"name": "  Иван Петров  ", "barcode": "00001234"}))
	f.olga = decode[api.Employee](h.t, h.post("/employees", map[string]any{"name": "Ольга Иванова", "barcode": "00009876"}))
	for _, n := range []int{1, 2} {
		f.cabinets = append(f.cabinets, decode[api.Cabinet](h.t, h.post("/cabinets", map[string]any{"number": n})))
	}
	for _, p := range [][2]int{{0, 1}, {0, 2}, {1, 1}} {
		f.shelves = append(f.shelves, decode[api.Shelf](h.t, h.post("/cabinets/"+f.cabinets[p[0]].ID+"/shelves", map[string]any{"number": p[1]})))
	}
	for _, p := range [][2]int{{0, 1}, {0, 2}, {1, 1}, {2, 1}} {
		f.cells = append(f.cells, decode[api.Cell](h.t, h.post("/shelves/"+f.shelves[p[0]].ID+"/cells", map[string]any{"number": p[1]})))
	}
	return f
}
func (h *harness) register(f fixture, code string, cell int) api.CommandResult {
	h.t.Helper()
	return decode[api.CommandResult](h.t, h.post("/batteries", map[string]any{"inventory_code": code, "cell_id": f.cells[cell].ID, "employee_barcode": "00001234"}))
}
func (h *harness) action(id, action, barcode, target string) response {
	h.t.Helper()
	body := map[string]any{"employee_barcode": barcode}
	if target != "" {
		if action == "loss" {
			body["reason"] = target
		} else {
			body["target_cell_id"] = target
		}
	}
	return h.post("/batteries/"+id+"/"+action, body)
}
func (h *harness) battery(id string) api.Battery {
	h.t.Helper()
	return decode[api.Battery](h.t, h.get("/batteries/"+id))
}
func (h *harness) operations(query string) []api.Operation {
	h.t.Helper()
	var page struct {
		Items []api.Operation `json:"items"`
	}
	page = decode[struct {
		Items []api.Operation `json:"items"`
	}](h.t, h.get("/operations"+query))
	return page.Items
}

func TestIntegrationT01T02T04T08T13T14T15T17_AllRoutes(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	if f.ivan.ActiveCredential.Barcode != "00001234" || f.ivan.Name != "Иван Петров" {
		t.Fatal("name normalization or leading zeros lost")
	}
	for i, want := range []string{"1.1.1", "1.1.2", "1.2.1", "2.1.1"} {
		if f.cells[i].Address != want {
			t.Fatalf("cell address %s want %s", f.cells[i].Address, want)
		}
	}
	reg := h.register(f, "AKB-0001", 0)
	h.register(f, "AKB-0002", 1)
	id := reg.Battery.ID
	path := "/batteries/" + id
	key := uuid.NewString()
	body := map[string]any{"employee_barcode": "00001234"}
	checkout := h.request("POST", path+"/checkout", body, key, 201)
	co := decode[api.CommandResult](t, checkout)
	if strptr(co.Battery.HolderEmployeeID) != f.ivan.ID || co.Battery.Version != 2 || co.Operation.ActorEmployeeID != f.ivan.ID || strptr(co.Operation.ToHolderEmployeeID) != f.ivan.ID {
		t.Fatal("incorrect checkout state/actor")
	}
	returned := decode[api.CommandResult](t, h.action(id, "return", "00009876", f.cells[2].ID))
	if returned.Operation.ActorEmployeeID != f.olga.ID || strptr(returned.Operation.FromHolderEmployeeID) != f.ivan.ID || returned.Battery.Version != 3 {
		t.Fatal("return actor confused with previous holder")
	}
	equalJSON(t, checkout, h.request("POST", path+"/checkout", body, key, 201))
	if current := h.battery(id); current.Status != "stored" || current.Version != 3 {
		t.Fatal("replay applied checkout twice")
	}
	moved := decode[api.CommandResult](t, h.action(id, "move", "00001234", f.cells[3].ID))
	if moved.Battery.Version != 4 {
		t.Fatal("move version")
	}
	lost := decode[api.CommandResult](t, h.action(id, "loss", "00001234", "  Не обнаружена при проверке  "))
	if lost.Battery.Status != "lost" || lost.Battery.Version != 5 || lost.Battery.CellID != nil || lost.Battery.HolderEmployeeID != nil || strptr(lost.Operation.FromCellID) != f.cells[3].ID || strptr(lost.Operation.Reason) != "Не обнаружена при проверке" {
		t.Fatal("lost state or previous location incorrect")
	}
	replacement := decode[api.Credential](t, h.request("PUT", "/employees/"+f.ivan.ID+"/credential", map[string]any{"barcode": "00005678"}, uuid.NewString(), 200))
	if replacement.EmployeeID != f.ivan.ID || replacement.Barcode != "00005678" || replacement.ID == f.ivan.ActiveCredential.ID {
		t.Fatal("credential replacement lost identity")
	}
	if h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1", f.ivan.ID) != 2 || h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1 AND revoked_at IS NULL", f.ivan.ID) != 1 {
		t.Fatal("credential history invariant")
	}
	for _, code := range []string{"00001234", "00009876"} {
		errorCode(t, h.request("PUT", "/employees/"+f.ivan.ID+"/credential", map[string]any{"barcode": code}, uuid.NewString(), 409), "BARCODE_EXISTS")
	}
	employee := decode[api.Employee](t, h.get("/employees/"+f.ivan.ID))
	if employee.ActiveCredential.ID != replacement.ID {
		t.Fatal("failed replacement revoked active credential")
	}
	noChange := decode[api.Credential](t, h.request("PUT", "/employees/"+f.ivan.ID+"/credential", map[string]any{"barcode": "00005678"}, uuid.NewString(), 200))
	if noChange.ID != replacement.ID {
		t.Fatal("same barcode created unnecessary credential")
	}
	equalJSON(t, checkout, h.request("POST", path+"/checkout", body, key, 201))
	errorCode(t, h.request("POST", "/batteries", map[string]any{"inventory_code": "OLD-CARD", "cell_id": f.cells[0].ID, "employee_barcode": "00001234"}, uuid.NewString(), 404), "EMPLOYEE_NOT_FOUND")
	ops := h.operations("?battery_id=" + id)
	if len(ops) != 5 {
		t.Fatalf("history has %d rows want 5", len(ops))
	}
	for i, want := range []string{"loss", "move", "return", "checkout", "register"} {
		if ops[i].Type != want || ops[i].BatteryVersion != int32(5-i) {
			t.Fatalf("bad history order: %+v", ops)
		}
	}
	if ops[3].CredentialID != f.ivan.ActiveCredential.ID {
		t.Fatal("old operation changed credential")
	}
	employeeOps := h.operations("?employee_id=" + f.ivan.ID)
	if len(employeeOps) != 6 {
		t.Fatalf("OR employee filter duplicated/missed rows: got %d want 6", len(employeeOps))
	}
	cellOps := h.operations("?cell_id=" + f.cells[3].ID)
	if len(cellOps) != 2 {
		t.Fatalf("cell filter has %d rows", len(cellOps))
	}
	at := url.QueryEscape(co.Operation.OccurredAt.Format(time.RFC3339Nano))
	from := h.operations("?battery_id=" + id + "&from=" + at)
	to := h.operations("?battery_id=" + id + "&to=" + at)
	if len(from) != 4 || len(to) != 1 {
		t.Fatalf("time bounds from inclusive/to exclusive failed: %d %d", len(from), len(to))
	}
	op := decode[api.Operation](t, h.get("/operations/"+lost.Operation.ID))
	if op.ID != lost.Battery.LastOperationID {
		t.Fatal("last operation link wrong")
	}
	for _, p := range []string{"/employees", "/cabinets", "/cabinets/" + f.cabinets[0].ID + "/shelves", "/cells", "/batteries", "/batteries?status=lost", "/batteries?cell_id=" + f.cells[1].ID, "/batteries?holder_employee_id=" + f.ivan.ID} {
		h.get(p)
	}
	h.get("/employees?limit=1&offset=1")
	h.get("/cells?shelf_id=" + f.shelves[0].ID + "&occupied=false")
	h.contract.assertAllOperations(t)
}

func TestIntegrationT03T09T18T19_ValidationAndAtomicErrors(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	reg := h.register(f, "FIRST", 0)
	failedKey := uuid.NewString()
	body := map[string]any{"inventory_code": "SECOND", "cell_id": f.cells[0].ID, "employee_barcode": "00001234"}
	errorCode(t, h.request("POST", "/batteries", body, failedKey, 409), "CELL_OCCUPIED")
	if h.count("SELECT count(*) FROM batteries") != 1 || h.count("SELECT count(*) FROM operations") != 1 || h.count("SELECT count(*) FROM idempotency_records WHERE key=$1", failedKey) != 0 {
		t.Fatal("rejected registration left partial changes")
	}
	body["cell_id"] = f.cells[1].ID
	h.request("POST", "/batteries", body, failedKey, 201) // Failed keys can be corrected and reused.
	errorCode(t, h.request("POST", "/batteries", body, uuid.NewString(), 409), "INVENTORY_CODE_EXISTS")
	key := uuid.NewString()
	original := h.request("POST", "/employees", map[string]any{"name": "Canonical", "barcode": "0000000"}, key, 201)
	equalJSON(t, original, h.raw("POST", "/employees", `{ "barcode":"0000000", "name":"  Canonical  " }`, key, "application/json; charset=utf-8", 201))
	errorCode(t, h.request("POST", "/employees", map[string]any{"name": "Changed", "barcode": "0000000"}, key, 409), "IDEMPOTENCY_CONFLICT")
	errorCode(t, h.request("POST", "/cabinets", map[string]any{"number": 99}, key, 409), "IDEMPOTENCY_CONFLICT")
	before := h.count("SELECT count(*) FROM idempotency_records")
	tests := []struct {
		method, path, body, key, content string
		status                           int
		code                             string
	}{
		{"POST", "/employees", `{"name":"A","barcode":"A","unknown":1}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":"A","name":"B","barcode":"A"}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":null,"barcode":"A"}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `null`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":"A"}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":"A","barcode":" A "}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":"A","barcode":123}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":"A","barcode":"A"} {}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees?x=1", `{"name":"A","barcode":"A"}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"GET", "/employees/not-uuid", ``, "", "", 400, "VALIDATION_ERROR"},
		{"POST", "/batteries", `{"inventory_code":"X","cell_id":"bad","employee_barcode":"00001234"}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/cabinets", `{"number":-1}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/cabinets", `{"number":1.5}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/cabinets", `{"number":2147483648}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/batteries/" + reg.Battery.ID + "/loss", `{"employee_barcode":"00001234","reason":"  "}`, "k", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":"A","barcode":"A"}`, "", "application/json", 400, "VALIDATION_ERROR"},
		{"PUT", "/employees/" + f.ivan.ID + "/credential", `{"barcode":"A"}`, "", "application/json", 400, "VALIDATION_ERROR"},
		{"POST", "/employees", `{"name":"A","barcode":"A"}`, "k", "text/plain", 415, "UNSUPPORTED_MEDIA_TYPE"},
		{"POST", "/employees", strings.Repeat(" ", 65537), "k", "application/json", 413, "REQUEST_TOO_LARGE"},
		{"GET", "/employees?limit=0", ``, "", "", 400, "VALIDATION_ERROR"},
		{"GET", "/employees?limit=101", ``, "", "", 400, "VALIDATION_ERROR"},
		{"GET", "/employees?offset=-1", ``, "", "", 400, "VALIDATION_ERROR"},
		{"GET", "/operations?from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", ``, "", "", 400, "VALIDATION_ERROR"},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			r := h.raw(tc.method, tc.path, tc.body, tc.key, tc.content, tc.status)
			errorCode(t, r, tc.code)
		})
	}
	if h.count("SELECT count(*) FROM idempotency_records") != before {
		t.Fatal("invalid requests reserved keys")
	}
	if b := h.battery(reg.Battery.ID); b.Version != 1 || b.Status != "stored" {
		t.Fatal("invalid requests changed state")
	}
}

type concurrentCommand struct{ method, path, body, key string }

func (h *harness) parallel(a, b concurrentCommand) [2]response {
	h.t.Helper()
	commands := []concurrentCommand{a, b}
	var servers [2]*httptest.Server
	var ids [2]int
	for i := range servers {
		db := stdlib.OpenDB(*h.config)
		db.SetMaxOpenConns(1)
		defer db.Close()
		if err := db.QueryRow("SELECT pg_backend_pid()").Scan(&ids[i]); err != nil {
			h.t.Fatal(err)
		}
		servers[i] = httptest.NewServer(api.New(db))
		defer servers[i].Close()
	}
	if ids[0] == ids[1] {
		h.t.Fatal("concurrency requires independent PostgreSQL sessions")
	}
	var result [2]response
	var errs [2]error
	start := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(2)
	done.Add(2)
	for i := range commands {
		go func(i int) {
			defer done.Done()
			ready.Done()
			<-start
			c := commands[i]
			result[i], errs[i] = perform(servers[i].Client(), servers[i].URL, c.method, c.path, c.body, c.key, "application/json")
		}(i)
	}
	ready.Wait()
	close(start)
	done.Wait()
	for i := range errs {
		if errs[i] != nil {
			h.t.Fatal(errs[i])
		}
		h.contract.validate(h.t, result[i])
	}
	return result
}
func commandJSON(t *testing.T, method, path, key string, body any) concurrentCommand {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return concurrentCommand{method, path, string(data), key}
}
func oneWinner(t *testing.T, r [2]response, code string) int {
	t.Helper()
	winner := -1
	for i, v := range r {
		if v.status == 201 {
			if winner != -1 {
				t.Fatal("both concurrent commands succeeded")
			}
			winner = i
		} else {
			if v.status != 409 {
				t.Fatalf("unexpected status %d: %s", v.status, v.body)
			}
			errorCode(t, v, code)
		}
	}
	if winner == -1 {
		t.Fatal("neither command succeeded")
	}
	return winner
}

func TestIntegrationT05_ConcurrentCheckout(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	reg := h.register(f, "RACE", 0)
	path := "/batteries/" + reg.Battery.ID + "/checkout"
	r := h.parallel(commandJSON(t, "POST", path, uuid.NewString(), map[string]any{"employee_barcode": "00001234"}), commandJSON(t, "POST", path, uuid.NewString(), map[string]any{"employee_barcode": "00009876"}))
	winner := oneWinner(t, r, "INVALID_TRANSITION")
	win := decode[api.CommandResult](t, r[winner])
	current := h.battery(reg.Battery.ID)
	if current.Version != 2 || strptr(current.HolderEmployeeID) != strptr(win.Battery.HolderEmployeeID) || h.count("SELECT count(*) FROM operations WHERE type='checkout'") != 1 {
		t.Fatal("double checkout or wrong holder")
	}
}
func TestIntegrationT06_ConcurrentPlacement(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	a := h.register(f, "A", 0)
	b := h.register(f, "B", 1)
	body := map[string]any{"employee_barcode": "00001234", "target_cell_id": f.cells[2].ID}
	r := h.parallel(commandJSON(t, "POST", "/batteries/"+a.Battery.ID+"/move", uuid.NewString(), body), commandJSON(t, "POST", "/batteries/"+b.Battery.ID+"/move", uuid.NewString(), body))
	win := oneWinner(t, r, "CELL_OCCUPIED")
	ids := []string{a.Battery.ID, b.Battery.ID}
	loser := h.battery(ids[1-win])
	if loser.Version != 1 || strptr(loser.CellID) != f.cells[1-win].ID {
		t.Fatal("losing move changed original location")
	}
	if h.count("SELECT count(*) FROM batteries WHERE cell_id=$1", f.cells[2].ID) != 1 || h.count("SELECT count(*) FROM operations WHERE type='move'") != 1 {
		t.Fatal("double occupied cell")
	}
}
func TestIntegrationT07_ConcurrentIdenticalKeyAndConflictingKey(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	reg := h.register(f, "REPLAY", 0)
	path := "/batteries/" + reg.Battery.ID + "/checkout"
	key := uuid.NewString()
	c := commandJSON(t, "POST", path, key, map[string]any{"employee_barcode": "00001234"})
	r := h.parallel(c, c)
	h.check(r[0], 201)
	h.check(r[1], 201)
	equalJSON(t, r[0], r[1])
	if h.count("SELECT count(*) FROM operations WHERE type='checkout'") != 1 || h.count("SELECT count(*) FROM idempotency_records WHERE key=$1", key) != 1 {
		t.Fatal("same key applied more than once")
	}
	conflictKey := uuid.NewString()
	r = h.parallel(commandJSON(t, "POST", "/employees", conflictKey, map[string]any{"name": "A", "barcode": "RACE-A"}), commandJSON(t, "POST", "/employees", conflictKey, map[string]any{"name": "B", "barcode": "RACE-B"}))
	oneWinner(t, r, "IDEMPOTENCY_CONFLICT")
}

func TestIntegrationT10_RollbackAfterBatteryUpdate(t *testing.T) {
	for _, target := range []string{"operations", "idempotency_records"} {
		t.Run(target, func(t *testing.T) {
			h := newHarness(t)
			f := h.fixture()
			reg := h.register(f, "ROLLBACK", 0)
			before := h.get("/batteries/" + reg.Battery.ID)
			key := uuid.NewString()
			event := "INSERT"
			if target == "idempotency_records" {
				event = "UPDATE"
			}
			_, err := h.owner.Exec(`CREATE FUNCTION force_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$; CREATE TRIGGER force_failure BEFORE ` + event + ` ON ` + target + ` FOR EACH ROW EXECUTE FUNCTION force_failure()`)
			if err != nil {
				t.Fatal(err)
			}
			errorCode(t, h.request("POST", "/batteries/"+reg.Battery.ID+"/checkout", map[string]any{"employee_barcode": "00001234"}, key, 500), "INTERNAL_ERROR")
			equalJSON(t, before, h.get("/batteries/"+reg.Battery.ID))
			if h.count("SELECT count(*) FROM operations") != 1 || h.count("SELECT count(*) FROM idempotency_records WHERE key=$1", key) != 0 {
				t.Fatal("transaction did not roll back history/key")
			}
			if _, err := h.owner.Exec("DROP TRIGGER force_failure ON " + target); err != nil {
				t.Fatal(err)
			}
			h.request("POST", "/batteries/"+reg.Battery.ID+"/checkout", map[string]any{"employee_barcode": "00001234"}, key, 201)
		})
	}
}

func TestIntegrationT11_LostHTTPResponseAfterCommit(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	reg := h.register(f, "DROPPED", 0)
	key := uuid.NewString()
	path := "/batteries/" + reg.Battery.ID + "/checkout"
	captured := make(chan response, 1)
	// ServeHTTP has committed before returning; hijacking closes the real TCP
	// connection without forwarding the buffered response to the HTTP client.
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
	_, err := perform(drop.Client(), drop.URL, "POST", path, `{"employee_barcode":"00001234"}`, key, "application/json")
	if err == nil {
		t.Fatal("client unexpectedly received dropped response")
	}
	original := <-captured
	h.check(original, 201)
	replay := h.request("POST", path, map[string]any{"employee_barcode": "00001234"}, key, 201)
	equalJSON(t, original, replay)
	if h.count("SELECT count(*) FROM operations") != 2 || h.battery(reg.Battery.ID).Version != 2 {
		t.Fatal("lost response caused second effect")
	}
}

func TestIntegrationT12T16_LossAndInvalidTransitions(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	a := h.register(f, "STORED", 0)
	b := h.register(f, "ISSUED", 1)
	invalid := func(id, action string, body any) {
		t.Helper()
		errorCode(t, h.request("POST", "/batteries/"+id+"/"+action, body, uuid.NewString(), 409), "INVALID_TRANSITION")
	}
	invalid(a.Battery.ID, "return", map[string]any{"employee_barcode": "00001234", "target_cell_id": f.cells[2].ID})
	invalid(a.Battery.ID, "move", map[string]any{"employee_barcode": "00001234", "target_cell_id": f.cells[0].ID})
	h.action(b.Battery.ID, "checkout", "00001234", "")
	invalid(b.Battery.ID, "move", map[string]any{"employee_barcode": "00001234", "target_cell_id": f.cells[2].ID})
	stored := decode[api.CommandResult](t, h.action(a.Battery.ID, "loss", "00009876", "Missing"))
	issued := decode[api.CommandResult](t, h.action(b.Battery.ID, "loss", "00009876", "Missing"))
	if strptr(stored.Operation.FromCellID) != f.cells[0].ID || stored.Operation.FromHolderEmployeeID != nil || strptr(issued.Operation.FromHolderEmployeeID) != f.ivan.ID || issued.Operation.FromCellID != nil {
		t.Fatal("loss did not preserve previous state")
	}
	for _, lost := range []api.CommandResult{stored, issued} {
		if lost.Battery.Status != "lost" || lost.Battery.CellID != nil || lost.Battery.HolderEmployeeID != nil {
			t.Fatal("lost battery still has current location")
		}
		invalid(lost.Battery.ID, "checkout", map[string]any{"employee_barcode": "00001234"})
		invalid(lost.Battery.ID, "loss", map[string]any{"employee_barcode": "00001234", "reason": "Again"})
	}
	if h.count("SELECT count(*) FROM operations") != 5 {
		t.Fatal("invalid transition created history")
	}
}

func TestIntegrationT20_DatabaseConstraintsAndAppendOnlyHistory(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	reg := h.register(f, "CONSTRAINT", 0)
	tests := []struct {
		sql, code string
		args      []any
	}{
		{`INSERT INTO batteries(id,inventory_code,status,cell_id,version) VALUES($1,'DUP','stored',$2,1)`, "23505", []any{uuid.NewString(), f.cells[0].ID}},
		{`INSERT INTO batteries(id,inventory_code,status,version) VALUES($1,'NO-HOLDER','issued',1)`, "23514", []any{uuid.NewString()}},
		{`INSERT INTO batteries(id,inventory_code,status,cell_id,version) VALUES($1,'NO-CELL','stored',$2,1)`, "23503", []any{uuid.NewString(), uuid.NewString()}},
		{`UPDATE operations SET credential_id=$1 WHERE id=$2`, "23503", []any{f.olga.ActiveCredential.ID, reg.Operation.ID}},
	}
	for _, tc := range tests {
		_, err := h.owner.Exec(tc.sql, tc.args...)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != tc.code {
			t.Fatalf("constraint: error %v want SQLSTATE %s", err, tc.code)
		}
	}
	tx, err := h.owner.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, q := range []string{`UPDATE batteries SET status='issued',cell_id=NULL,holder_employee_id=$2 WHERE id=$1`, `UPDATE batteries SET status='lost',cell_id=NULL,holder_employee_id=NULL WHERE id=$1`} {
		if _, err := tx.Exec(q, reg.Battery.ID, f.ivan.ID); err != nil { // second statement has only one placeholder
			if !strings.Contains(q, "$2") {
				if _, err = tx.Exec(q, reg.Battery.ID); err == nil {
					continue
				}
			}
			t.Fatal(err)
		}
	}
	if os.Getenv("TEST_APP_DATABASE_URL") == "" {
		t.Log("TEST_APP_DATABASE_URL absent: limited database-role privileges not checked")
		return
	}
	for _, query := range []string{"UPDATE operations SET reason=reason", "DELETE FROM operations", "TRUNCATE operations", "DELETE FROM batteries", "SELECT * FROM schema_migrations"} {
		_, err := h.app.Exec(query)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "42501" {
			t.Fatalf("app unexpectedly allowed %q: %v", query, err)
		}
	}
}

func TestIntegrationConcurrentCredentialReplacement(t *testing.T) {
	h := newHarness(t)
	f := h.fixture()
	path := "/employees/" + f.ivan.ID + "/credential"
	r := h.parallel(commandJSON(t, "PUT", path, uuid.NewString(), map[string]any{"barcode": "NEW-A"}), commandJSON(t, "PUT", path, uuid.NewString(), map[string]any{"barcode": "NEW-B"}))
	h.check(r[0], 200)
	h.check(r[1], 200)
	if h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1", f.ivan.ID) != 3 || h.count("SELECT count(*) FROM employee_credentials WHERE employee_id=$1 AND revoked_at IS NULL", f.ivan.ID) != 1 {
		t.Fatal("concurrent replacement violated credential uniqueness/history")
	}
	active := decode[api.Employee](t, h.get("/employees/"+f.ivan.ID)).ActiveCredential
	if active.Barcode != "NEW-A" && active.Barcode != "NEW-B" {
		t.Fatal("neither replacement became active")
	}
}
