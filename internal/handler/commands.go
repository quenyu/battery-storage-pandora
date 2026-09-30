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

func (s *Server) executeCommand(w http.ResponseWriter, r *http.Request, body requestBody,
	execute func(context.Context, model.CommandRequest) (model.CommandResponse, error),
) {
	canonicalBody, err := canonicalJSON(body)
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
	result, err := execute(ctx, request)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Idempotency-Replayed", fmt.Sprint(result.Replayed))
	setLocation(w, r, result.Status, result.Body)
	writeJSON(w, result.Status, json.RawMessage(result.Body))
}

// Keep the old hash format: sorted fields, absent optional fields omitted.
// This lets requests saved before the refactor be replayed unchanged.
func canonicalJSON(body requestBody) ([]byte, error) {
	fields, err := jsonFields(body)
	if err != nil {
		return nil, err
	}
	for field, value := range fields {
		if string(value) == "null" {
			delete(fields, field)
		}
	}
	return json.Marshal(fields)
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
