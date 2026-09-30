package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type command func(context.Context, model.CommandRequest, *http.Request, input) (model.CommandResponse, error)

func (s *Server) command(fields string, execute command) http.HandlerFunc {
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
		canonicalBody, err := json.Marshal(values)
		if err != nil {
			writeError(w, err)
			return
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r.Method+"\n"+canonicalPath(r)+"\n"+string(canonicalBody))))
		request := model.CommandRequest{
			Key:       r.Header.Get("Idempotency-Key"),
			Hash:      hash,
			RequestID: w.Header().Get("X-Request-ID"),
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		result, err := execute(ctx, request, r, values)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Idempotency-Replayed", fmt.Sprint(result.Replayed))
		setLocation(w, r, result.Status, result.Body)
		writeJSON(w, result.Status, json.RawMessage(result.Body))
	}
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
