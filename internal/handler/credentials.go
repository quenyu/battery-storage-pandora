package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

type createCredentialRequest struct {
	Value                *string `json:"value"`
	ReplacesCredentialID *int64  `json:"replaces_credential_id"`
}

type disableCredentialRequest struct {
	IsActive *bool `json:"is_active"`
}

type resolveCredentialRequest struct {
	CredentialValue *string `json:"credential_value"`
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	employeeID, err := pathID(r, "employee_id")
	if err != nil {
		writeError(w, err)
		return
	}
	var body createCredentialRequest
	if err := decodeRequest(w, r, &body, "value replaces_credential_id"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.Value); err != nil {
		writeError(w, err)
		return
	}
	if body.ReplacesCredentialID != nil && *body.ReplacesCredentialID < 1 {
		writeError(w, model.Invalid("Неверный идентификатор карты"))
		return
	}

	input := model.CreateCredentialInput{Value: *body.Value, ReplacesCredentialID: body.ReplacesCredentialID}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.CreateCredential(ctx, employeeID, input)
	})
}

func (s *Server) disableCredential(w http.ResponseWriter, r *http.Request) {
	employeeID, err := pathID(r, "employee_id")
	if err != nil {
		writeError(w, err)
		return
	}
	credentialID, err := pathID(r, "credential_id")
	if err != nil {
		writeError(w, err)
		return
	}
	var body disableCredentialRequest
	if err := decodeRequest(w, r, &body, "is_active"); err != nil {
		writeError(w, err)
		return
	}
	if body.IsActive == nil {
		writeError(w, model.Invalid("Отсутствует обязательное поле"))
		return
	}

	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.DisableCredential(ctx, employeeID, credentialID, *body.IsActive)
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
	ctx, cancel := readContext(r)
	defer cancel()
	result, err := s.service.ResolveCredential(ctx, *body.CredentialValue)
	writeReadResult(w, result, err)
}
