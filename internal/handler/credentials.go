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

func (body *createCredentialRequest) validate() error {
	if err := requiredText(body.Value); err != nil {
		return err
	}
	if err := optionalText(body.ReplacesCredentialID); err != nil {
		return err
	}
	if body.ReplacesCredentialID != nil {
		if !model.ValidUUID(*body.ReplacesCredentialID) {
			return model.Invalid("Неверный UUID")
		}
		*body.ReplacesCredentialID = strings.ToLower(*body.ReplacesCredentialID)
	}
	return nil
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	var body createCredentialRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	input := model.CreateCredentialInput{Value: *body.Value, ReplacesCredentialID: body.ReplacesCredentialID}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.CreateCredential(ctx, request, r.PathValue("employee_id"), input)
	})
}

type disableCredentialRequest struct {
	IsActive *bool `json:"is_active"`
}

func (body *disableCredentialRequest) validate() error {
	if body.IsActive == nil {
		return model.Invalid("Отсутствует обязательное поле")
	}
	return nil
}

func (s *Server) disableCredential(w http.ResponseWriter, r *http.Request) {
	var body disableCredentialRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.DisableCredential(ctx, request, r.PathValue("employee_id"), r.PathValue("credential_id"), *body.IsActive)
	})
}

type resolveCredentialRequest struct {
	CredentialValue *string `json:"credential_value"`
}

func (body *resolveCredentialRequest) validate() error {
	return requiredText(body.CredentialValue)
}

func (s *Server) resolveCredential(w http.ResponseWriter, r *http.Request) {
	var body resolveCredentialRequest
	if err := decodeRequest(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ResolveCredential(ctx, *body.CredentialValue)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
