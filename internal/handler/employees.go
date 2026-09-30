package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

// Pointers distinguish a missing field from an explicitly empty value.
type createEmployeeRequest struct {
	DisplayName     *string `json:"display_name"`
	PersonnelNumber *string `json:"personnel_number"`
}

func (body *createEmployeeRequest) validate() error {
	if err := requiredText(body.DisplayName); err != nil {
		return err
	}
	return optionalText(body.PersonnelNumber)
}

func (s *Server) createEmployee(w http.ResponseWriter, r *http.Request) {
	var body createEmployeeRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	input := model.CreateEmployeeInput{DisplayName: *body.DisplayName, PersonnelNumber: body.PersonnelNumber}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.CreateEmployee(ctx, request, input)
	})
}

type patchEmployeeRequest struct {
	DisplayName *string `json:"display_name"`
	IsActive    *bool   `json:"is_active"`
}

func (body *patchEmployeeRequest) validate() error {
	return optionalText(body.DisplayName)
}

func (s *Server) patchEmployee(w http.ResponseWriter, r *http.Request) {
	var body patchEmployeeRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	input := model.PatchEmployeeInput{DisplayName: body.DisplayName, IsActive: body.IsActive}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.PatchEmployee(ctx, request, r.PathValue("employee_id"), input)
	})
}
