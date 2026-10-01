package handler

import (
	"battery-storage-pandora/internal/model"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
)

func commandRequest(w http.ResponseWriter, r *http.Request, body any) (model.CommandRequest, error) {
	canonical, err := canonicalJSON(body)
	if err != nil {
		return model.CommandRequest{}, err
	}
	hash := sha256.Sum256([]byte(r.Method + "\n" + canonicalPath(r) + "\n" + string(canonical)))
	return model.CommandRequest{
		Key:       r.Header.Get("Idempotency-Key"),
		Hash:      fmt.Sprintf("%x", hash),
		RequestID: w.Header().Get("X-Request-ID"),
	}, nil
}

// Preserve saved hashes: sorted fields, absent optional fields omitted, exact int64.
func canonicalJSON(body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for name, value := range fields {
		if string(value) == "null" {
			delete(fields, name)
		}
	}
	return json.Marshal(fields)
}

func writeCommand(w http.ResponseWriter, r *http.Request, result model.CommandResponse, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Idempotency-Replayed", fmt.Sprint(result.Replayed))
	if result.Status == http.StatusCreated {
		var resource struct {
			ID        string `json:"id"`
			Operation struct {
				ID string `json:"id"`
			} `json:"operation"`
		}
		if json.Unmarshal(result.Body, &resource) == nil {
			if resource.Operation.ID != "" {
				w.Header().Set("Location", "/api/operations/"+resource.Operation.ID)
			} else if resource.ID != "" {
				w.Header().Set("Location", canonicalPath(r)+"/"+resource.ID)
			}
		}
	}
	writeJSON(w, result.Status, json.RawMessage(result.Body))
}
