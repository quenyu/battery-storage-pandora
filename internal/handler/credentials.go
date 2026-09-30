package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
	"time"
)

func (s *Server) createCredential(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.CreateCredential(ctx, request, r.PathValue("employee_id"), model.CreateCredentialInput{
		Value:                values.str("value"),
		ReplacesCredentialID: values.optionalString("replaces_credential_id"),
	})
}

func (s *Server) disableCredential(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.DisableCredential(ctx, request, r.PathValue("employee_id"),
		r.PathValue("credential_id"), values["is_active"].(bool))
}

func (s *Server) resolveCredential(w http.ResponseWriter, r *http.Request) {
	values, err := decodeInput(w, r, "credential_value", false)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ResolveCredential(ctx, values.str("credential_value"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
