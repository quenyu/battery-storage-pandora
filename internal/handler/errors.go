package handler

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/service"
	"net/http"
)

func classify(err error) *model.Error { return service.ClassifyError(err) }

func writeError(w http.ResponseWriter, err error) {
	apiError := classify(err)
	writeJSON(w, apiError.Status, model.ErrorBody(apiError, w.Header().Get("X-Request-ID")))
}
