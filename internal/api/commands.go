package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// The existing schema uses a scope column. All requests share one namespace.
const requestScope = "pandora"

type command func(context.Context, *sql.Tx, *http.Request, input) (any, error)

type commandResult struct {
	status   int
	body     []byte
	replayed bool
}

func (s *Server) command(fields string, status int, execute command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := normalizePath(r); err != nil {
			writeError(w, err)
			return
		}
		values, err := decodeInput(w, r, fields, true)
		if err != nil {
			writeError(w, err)
			return
		}
		result, err := s.executeCommand(r, values, status, execute)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Idempotency-Replayed", fmt.Sprint(result.replayed))
		setLocation(w, r, result.status, result.body)
		writeJSON(w, result.status, json.RawMessage(result.body))
	}
}

func (s *Server) executeCommand(r *http.Request, values input, status int, execute command) (commandResult, error) {
	canonicalBody, err := json.Marshal(values)
	if err != nil {
		return commandResult{}, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r.Method+"\n"+canonicalPath(r)+"\n"+string(canonicalBody))))
	key := r.Header.Get("Idempotency-Key")

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return commandResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
        SET LOCAL lock_timeout = '5s';
        SET LOCAL statement_timeout = '10s';
        SET LOCAL TIME ZONE 'UTC'
    `); err != nil {
		return commandResult{}, err
	}

	reservation, err := tx.ExecContext(ctx, `
        INSERT INTO idempotency_requests (scope, key, request_hash)
        VALUES ($1, $2, $3)
        ON CONFLICT DO NOTHING
    `, requestScope, key, hash)
	if err != nil {
		return commandResult{}, err
	}
	inserted, err := reservation.RowsAffected()
	if err != nil {
		return commandResult{}, err
	}
	if inserted == 0 {
		return savedCommand(ctx, tx, key, hash)
	}

	// Roll back a business conflict while retaining its original response.
	if _, err := tx.ExecContext(ctx, `SAVEPOINT business`); err != nil {
		return commandResult{}, err
	}
	value, businessError := execute(ctx, tx, r, values)
	if businessError != nil {
		apiError := classify(businessError)
		if apiError.Status != 404 && apiError.Status != 409 && apiError.Status != 422 {
			return commandResult{}, apiError
		}
		if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT business`); err != nil {
			return commandResult{}, err
		}
		status = apiError.Status
		value = errorBody(apiError, r.Header.Get("X-Request-ID"))
	}
	body, err := json.Marshal(value)
	if err != nil {
		return commandResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
        UPDATE idempotency_requests
        SET http_status = $3, response_body = $4
        WHERE scope = $1 AND key = $2
    `, requestScope, key, status, string(body)); err != nil {
		return commandResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return commandResult{}, fail(503, "TEMPORARILY_UNAVAILABLE", "Результат фиксации неизвестен; повторите тот же ключ")
	}
	return commandResult{status: status, body: body}, nil
}

func savedCommand(ctx context.Context, tx *sql.Tx, key, hash string) (commandResult, error) {
	var savedHash string
	result := commandResult{replayed: true}
	err := tx.QueryRowContext(ctx, `
        SELECT request_hash, http_status, response_body
        FROM idempotency_requests
        WHERE scope = $1 AND key = $2
    `, requestScope, key).Scan(&savedHash, &result.status, &result.body)
	if err != nil {
		return commandResult{}, err
	}
	if savedHash != hash {
		return commandResult{}, conflict("IDEMPOTENCY_KEY_REUSED")
	}
	return result, nil
}

func setLocation(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	if status != http.StatusCreated {
		return
	}
	var resource struct {
		ID        string `json:"id"`
		Operation struct {
			ID string `json:"id"`
		} `json:"operation"`
	}
	if json.Unmarshal(body, &resource) != nil {
		return
	}
	if resource.Operation.ID != "" {
		w.Header().Set("Location", "/api/operations/"+resource.Operation.ID)
	} else if resource.ID != "" {
		w.Header().Set("Location", canonicalPath(r)+"/"+resource.ID)
	}
}
