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
	"net/http"
	"strings"
	"time"
)

type Server struct{ db *sql.DB }

func New(db *sql.DB) http.Handler {
	s := &Server{db: db}
	mux := http.NewServeMux()
	ui := swagger.Handler()
	mux.Handle("GET /swagger", ui)
	mux.Handle("GET /swagger/", ui)
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(assets.OpenAPI)
	})
	s.routes(mux)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type command func(context.Context, *sql.Tx, *http.Request, input) (any, error)

func (s *Server) command(fields string, status int, fn command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := normalizePath(r); err != nil {
			writeError(w, err)
			return
		}
		in, err := decodeInput(w, r, fields)
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
		key := r.Header.Get("Idempotency-Key")
		result, err := tx.ExecContext(ctx, `INSERT INTO idempotency_records(key,request_hash) VALUES ($1,$2) ON CONFLICT DO NOTHING`, key, hash)
		if err != nil {
			writeError(w, err)
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			var oldHash string
			var oldStatus int
			var body []byte
			err = tx.QueryRowContext(ctx, `SELECT request_hash,http_status,response_body FROM idempotency_records WHERE key=$1`, key).Scan(&oldHash, &oldStatus, &body)
			if err != nil {
				writeError(w, err)
				return
			}
			if hash != oldHash {
				writeError(w, conflict("IDEMPOTENCY_CONFLICT"))
				return
			}
			writeJSON(w, oldStatus, json.RawMessage(body))
			return
		}
		value, err := fn(ctx, tx, r, in)
		if err != nil {
			writeError(w, err)
			return
		}
		body, err := json.Marshal(value)
		if err != nil {
			writeError(w, err)
			return
		}
		if _, err = tx.ExecContext(ctx, `UPDATE idempotency_records SET http_status=$2,response_body=$3 WHERE key=$1`, key, status, string(body)); err != nil {
			writeError(w, err)
			return
		}
		if err = tx.Commit(); err != nil {
			writeError(w, fail(503, "TEMPORARILY_UNAVAILABLE", "Результат фиксации неизвестен; повторите тот же ключ"))
			return
		}
		writeJSON(w, status, json.RawMessage(body))
	}
}
func normalizePath(r *http.Request) error {
	for _, key := range []string{"employee_id", "cabinet_id", "shelf_id", "battery_id", "operation_id"} {
		value := r.PathValue(key)
		if value == "" {
			continue
		}
		if !uuidPattern.MatchString(value) {
			return invalid("Неверный UUID в пути")
		}
		r.SetPathValue(key, strings.ToLower(value))
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

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/employees", s.command("name barcode", 201, createEmployee))
	mux.HandleFunc("GET /api/v1/employees", s.read(s.listEmployees))
	mux.HandleFunc("GET /api/v1/employees/{employee_id}", s.read(func(ctx context.Context, r *http.Request) (any, error) {
		return getEmployee(ctx, s.db, r.PathValue("employee_id"))
	}))
	mux.HandleFunc("PUT /api/v1/employees/{employee_id}/credential", s.command("barcode", 200, replaceCredential))
	mux.HandleFunc("POST /api/v1/cabinets", s.command("number", 201, createCabinet))
	mux.HandleFunc("GET /api/v1/cabinets", s.read(s.listCabinets))
	mux.HandleFunc("POST /api/v1/cabinets/{cabinet_id}/shelves", s.command("number", 201, createShelf))
	mux.HandleFunc("GET /api/v1/cabinets/{cabinet_id}/shelves", s.read(s.listShelves))
	mux.HandleFunc("POST /api/v1/shelves/{shelf_id}/cells", s.command("number", 201, createCell))
	mux.HandleFunc("GET /api/v1/shelves/{shelf_id}/cells", s.read(s.listCells))
	mux.HandleFunc("POST /api/v1/batteries", s.command("inventory_code cell_id employee_barcode", 201, registerBattery))
	mux.HandleFunc("GET /api/v1/batteries", s.read(s.listBatteries))
	mux.HandleFunc("GET /api/v1/batteries/{battery_id}", s.read(func(ctx context.Context, r *http.Request) (any, error) {
		return getBattery(ctx, s.db, r.PathValue("battery_id"))
	}))
	for _, kind := range []string{"checkout", "return", "move", "loss"} {
		fields := "employee_barcode"
		if kind == "return" || kind == "move" {
			fields += " target_cell_id"
		}
		if kind == "loss" {
			fields += " reason"
		}
		mux.HandleFunc("POST /api/v1/batteries/{battery_id}/"+kind, s.command(fields, 201, batteryCommand(kind)))
	}
	mux.HandleFunc("GET /api/v1/operations", s.read(s.listOperations))
	mux.HandleFunc("GET /api/v1/operations/{operation_id}", s.read(func(ctx context.Context, r *http.Request) (any, error) {
		return getOperation(ctx, s.db, r.PathValue("operation_id"))
	}))
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
type scanner interface{ Scan(...any) error }

const employeeSelect = `SELECT e.id,e.name,e.created_at,c.id,c.employee_id,c.barcode,c.created_at,c.revoked_at FROM employees e JOIN employee_credentials c ON c.employee_id=e.id AND c.revoked_at IS NULL`

func scanEmployee(row scanner) (Employee, error) {
	var e Employee
	c := &e.ActiveCredential
	err := row.Scan(&e.ID, &e.Name, &e.CreatedAt, &c.ID, &c.EmployeeID, &c.Barcode, &c.CreatedAt, &c.RevokedAt)
	e.CreatedAt = e.CreatedAt.UTC()
	c.CreatedAt = c.CreatedAt.UTC()
	return e, err
}
func getEmployee(ctx context.Context, q queryer, id string) (Employee, error) {
	return scanEmployee(q.QueryRowContext(ctx, employeeSelect+` WHERE e.id=$1`, id))
}
func createEmployee(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	id := newID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO employees(id,name) VALUES($1,$2)`, id, in.str("name")); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO employee_credentials(id,employee_id,barcode) VALUES($1,$2,$3)`, newID(), id, in.str("barcode")); err != nil {
		return nil, err
	}
	return getEmployee(ctx, tx, id)
}
func (s *Server) read(fn func(context.Context, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := normalizePath(r); err != nil {
			writeError(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		value, err := fn(ctx, r)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, value)
	}
}
