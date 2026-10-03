package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
	"strings"
	"time"
)

type createCredentialRequest struct {
	Value                *string `json:"value"`
	ReplacesCredentialID *string `json:"replaces_credential_id"`
}

type disableCredentialRequest struct {
	IsActive *bool `json:"is_active"`
}

type resolveCredentialRequest struct {
	CredentialValue *string `json:"credential_value"`
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	var body createCredentialRequest
	if err := decodeCommand(w, r, &body, "value replaces_credential_id"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.Value); err != nil {
		writeError(w, err)
		return
	}
	if err := optionalText(body.ReplacesCredentialID); err != nil {
		writeError(w, err)
		return
	}
	if body.ReplacesCredentialID != nil {
		if !model.ValidUUID(*body.ReplacesCredentialID) {
			writeError(w, model.Invalid("Неверный UUID карты"))
			return
		}
		*body.ReplacesCredentialID = strings.ToLower(*body.ReplacesCredentialID)
	}

	input := model.CreateCredentialInput{Value: *body.Value, ReplacesCredentialID: body.ReplacesCredentialID}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.CreateCredential(ctx, r.PathValue("employee_id"), input)
	})
}

func (s *Server) disableCredential(w http.ResponseWriter, r *http.Request) {
	var body disableCredentialRequest
	if err := decodeCommand(w, r, &body, "is_active"); err != nil {
		writeError(w, err)
		return
	}
	if body.IsActive == nil {
		writeError(w, model.Invalid("Отсутствует обязательное поле"))
		return
	}

	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.DisableCredential(ctx, r.PathValue("employee_id"), r.PathValue("credential_id"), *body.IsActive)
	})
}

func (s *Server) resolveCredential(w http.ResponseWriter, r *http.Request) {
	var body resolveCredentialRequest
	if err := decodeRequest(w, r, &body, "credential_value"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.CredentialValue); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ResolveCredential(ctx, *body.CredentialValue)
	writeReadResult(w, result, err)
}
