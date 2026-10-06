package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// executeCommand finishes an already decoded and validated HTTP command.
func executeCommand(w http.ResponseWriter, r *http.Request, execute func(context.Context) (model.CommandResponse, error)) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := execute(ctx)
	writeCommand(w, r, result, err)
}

func writeCommand(w http.ResponseWriter, r *http.Request, result model.CommandResponse, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	if result.Status == http.StatusCreated {
		var resource struct {
			ID        int64 `json:"id"`
			Operation struct {
				ID int64 `json:"id"`
			} `json:"operation"`
		}
		if json.Unmarshal(result.Body, &resource) == nil {
			if resource.Operation.ID != 0 {
				w.Header().Set("Location", "/api/operations/"+strconv.FormatInt(resource.Operation.ID, 10))
			} else if resource.ID != 0 {
				w.Header().Set("Location", r.URL.Path+"/"+strconv.FormatInt(resource.ID, 10))
			}
		}
	}
	writeJSON(w, result.Status, json.RawMessage(result.Body))
}
