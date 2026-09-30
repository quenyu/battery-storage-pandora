package api

import (
	assets "battery-storage-pandora"
	"battery-storage-pandora/swagger"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	db           *sql.DB
	authenticate Authenticator
}
type principalKey struct{}

func New(db *sql.DB) http.Handler { return NewWithAuth(db, StaticTokens(nil)) }
func NewWithAuth(db *sql.DB, auth Authenticator) http.Handler {
	if auth == nil {
		auth = StaticTokens(nil)
	}
	s := &Server{db: db, authenticate: auth}
	mux := http.NewServeMux()
	ui := swagger.Handler()
	mux.Handle("GET /swagger", ui)
	mux.Handle("GET /swagger/", ui)
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(assets.OpenAPI)
	})
	s.routes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, notFound()) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", newID())
		start := time.Now()
		recorder := &statusWriter{ResponseWriter: w}
		mux.ServeHTTP(recorder, r)
		slog.Info("request", "request_id", w.Header().Get("X-Request-ID"), "method", r.Method, "route", r.Pattern, "status", recorder.status, "duration_ms", time.Since(start).Milliseconds(), "replay", w.Header().Get("Idempotency-Replayed"))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) permit(right string, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authenticate(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if strings.TrimSpace(p.Scope) == "" {
			writeError(w, fail(401, "UNAUTHENTICATED", "Требуется аутентификация"))
			return
		}
		allowed := right == "read" && p.Read || right == "manage" && p.Manage || right == "command" && p.Command || right == "register" && p.Command && p.Manage
		if !allowed {
			writeError(w, fail(403, "FORBIDDEN", "Действие не разрешено"))
			return
		}
		fn(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	}
}

type command func(context.Context, *sql.Tx, *http.Request, input) (any, error)

func (s *Server) command(fields string, status int, fn command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := normalizePath(r); err != nil {
			writeError(w, err)
			return
		}
		in, err := decodeInput(w, r, fields, true)
		if err != nil {
			writeError(w, err)
			return
		}
		canonical, _ := json.Marshal(in)
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r.Method+"\n"+canonicalPath(r)+"\n"+string(canonical))))
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			writeError(w, err)
			return
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='10s'; SET LOCAL TIME ZONE 'UTC'`); err != nil {
			writeError(w, err)
			return
		}
		p := r.Context().Value(principalKey{}).(Principal)
		key := r.Header.Get("Idempotency-Key")
		res, err := tx.ExecContext(ctx, `INSERT INTO idempotency_requests(scope,key,request_hash) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, p.Scope, key, hash)
		if err != nil {
			writeError(w, err)
			return
		}
		count, err := res.RowsAffected()
		if err != nil {
			writeError(w, err)
			return
		}
		if count == 0 {
			var oldHash string
			var oldStatus int
			var body []byte
			err = tx.QueryRowContext(ctx, `SELECT request_hash,http_status,response_body FROM idempotency_requests WHERE scope=$1 AND key=$2`, p.Scope, key).Scan(&oldHash, &oldStatus, &body)
			if err != nil {
				writeError(w, err)
				return
			}
			if oldHash != hash {
				writeError(w, conflict("IDEMPOTENCY_KEY_REUSED"))
				return
			}
			w.Header().Set("Idempotency-Replayed", "true")
			setLocation(w, r, oldStatus, body)
			writeJSON(w, oldStatus, json.RawMessage(body))
			return
		}
		if _, err = tx.ExecContext(ctx, `SAVEPOINT business`); err != nil {
			writeError(w, err)
			return
		}
		value, businessErr := fn(ctx, tx, r, in)
		responseStatus := status
		if businessErr != nil {
			a := classify(businessErr)
			if a.Status != 404 && a.Status != 409 && a.Status != 422 {
				writeError(w, a)
				return
			}
			if _, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT business`); err != nil {
				writeError(w, err)
				return
			}
			responseStatus = a.Status
			value = errorBody(a, w.Header().Get("X-Request-ID"))
		}
		body, err := json.Marshal(value)
		if err != nil {
			writeError(w, err)
			return
		}
		if _, err = tx.ExecContext(ctx, `UPDATE idempotency_requests SET http_status=$3,response_body=$4 WHERE scope=$1 AND key=$2`, p.Scope, key, responseStatus, string(body)); err != nil {
			writeError(w, err)
			return
		}
		if err = tx.Commit(); err != nil {
			writeError(w, fail(503, "TEMPORARILY_UNAVAILABLE", "Результат фиксации неизвестен; повторите тот же ключ"))
			return
		}
		w.Header().Set("Idempotency-Replayed", "false")
		setLocation(w, r, responseStatus, body)
		writeJSON(w, responseStatus, json.RawMessage(body))
	}
}
func setLocation(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	if status != 201 {
		return
	}
	var v struct {
		ID        string `json:"id"`
		Operation struct {
			ID string `json:"id"`
		} `json:"operation"`
	}
	if json.Unmarshal(body, &v) != nil {
		return
	}
	if v.Operation.ID != "" {
		w.Header().Set("Location", "/api/v1/operations/"+v.Operation.ID)
	} else if v.ID != "" {
		w.Header().Set("Location", canonicalPath(r)+"/"+v.ID)
	}
}
func normalizePath(r *http.Request) error {
	for _, k := range []string{"employee_id", "credential_id", "battery_id", "operation_id"} {
		if v := r.PathValue(k); v != "" {
			if !uuidPattern.MatchString(v) {
				return invalid("Неверный UUID в пути")
			}
			r.SetPathValue(k, strings.ToLower(v))
		}
	}
	return nil
}
func canonicalPath(r *http.Request) string {
	parts := strings.Split(r.URL.Path, "/")
	for i, p := range parts {
		if uuidPattern.MatchString(p) {
			parts[i] = strings.ToLower(p)
		}
	}
	return strings.Join(parts, "/")
}
func newID() string { return uuid.NewString() }

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
type scanner interface{ Scan(...any) error }

func (s *Server) read(fn func(context.Context, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := normalizePath(r); err != nil {
			writeError(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		v, err := fn(ctx, r)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, v)
	}
}
func (s *Server) routes(m *http.ServeMux) {
	register := func(methodPath, right string, h http.HandlerFunc) { m.HandleFunc(methodPath, s.permit(right, h)) }
	register("POST /api/v1/employees", "manage", s.command("display_name personnel_number?", 201, createEmployee))
	register("GET /api/v1/employees", "read", s.read(s.listEmployees))
	register("GET /api/v1/employees/{employee_id}", "read", s.read(func(ctx context.Context, r *http.Request) (any, error) {
		if err := noQuery(r); err != nil {
			return nil, err
		}
		return getEmployee(ctx, s.db, r.PathValue("employee_id"))
	}))
	register("PATCH /api/v1/employees/{employee_id}", "manage", s.command("display_name? is_active?", 200, patchEmployee))
	register("POST /api/v1/credential-resolutions", "read", s.resolveCredential)
	register("GET /api/v1/employees/{employee_id}/credentials", "manage", s.read(s.listCredentials))
	register("POST /api/v1/employees/{employee_id}/credentials", "manage", s.command("value replaces_credential_id?", 201, createCredential))
	register("PATCH /api/v1/employees/{employee_id}/credentials/{credential_id}", "manage", s.command("is_active", 200, disableCredential))
	register("GET /api/v1/batteries", "read", s.read(s.listBatteries))
	register("POST /api/v1/batteries", "register", s.command("inventory_code serial_number? actor_credential_value destination_location", 201, registerBattery))
	register("GET /api/v1/batteries/{battery_id}", "read", s.read(func(ctx context.Context, r *http.Request) (any, error) {
		if err := noQuery(r); err != nil {
			return nil, err
		}
		return getBattery(ctx, s.db, r.PathValue("battery_id"))
	}))
	for _, kind := range []string{"take", "return", "move"} {
		fields := "actor_credential_value expected_version?"
		if kind != "take" {
			fields += " destination_location"
		}
		if kind != "return" {
			fields += " observed_source_location?"
		}
		register("POST /api/v1/batteries/{battery_id}/"+kind, "command", s.command(fields, 201, batteryCommand(strings.ToUpper(kind))))
	}
	register("GET /api/v1/batteries/{battery_id}/operations", "read", s.read(s.listBatteryOperations))
	register("GET /api/v1/employees/{employee_id}/batteries", "read", s.read(s.listCustody))
	register("GET /api/v1/employees/{employee_id}/operations", "read", s.read(s.listEmployeeOperations))
	register("GET /api/v1/operations", "read", s.read(s.listOperations))
	register("GET /api/v1/operations/{operation_id}", "read", s.read(func(ctx context.Context, r *http.Request) (any, error) {
		if err := noQuery(r); err != nil {
			return nil, err
		}
		return getOperation(ctx, s.db, r.PathValue("operation_id"))
	}))
}
func noQuery(r *http.Request) error {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return invalid("Query-параметры не поддерживаются")
	}
	return nil
}
