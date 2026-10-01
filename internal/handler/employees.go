package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
	"time"
)

type createEmployeeRequest struct {
	DisplayName     *string `json:"display_name"`
	PersonnelNumber *string `json:"personnel_number"`
}

type patchEmployeeRequest struct {
	DisplayName *string `json:"display_name"`
	IsActive    *bool   `json:"is_active"`
}

func (s *Server) createEmployee(w http.ResponseWriter, r *http.Request) {
	var body createEmployeeRequest
	if err := decodeCommand(w, r, &body, "display_name personnel_number"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.DisplayName); err != nil {
		writeError(w, err)
		return
	}
	if err := optionalText(body.PersonnelNumber); err != nil {
		writeError(w, err)
		return
	}

	request, err := commandRequest(w, r, &body)
	if err != nil {
		writeError(w, err)
		return
	}
	input := model.CreateEmployeeInput{DisplayName: *body.DisplayName, PersonnelNumber: body.PersonnelNumber}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.CreateEmployee(ctx, request, input)
	writeCommand(w, r, result, err)
}

func (s *Server) patchEmployee(w http.ResponseWriter, r *http.Request) {
	var body patchEmployeeRequest
	if err := decodeCommand(w, r, &body, "display_name is_active"); err != nil {
		writeError(w, err)
		return
	}
	if err := optionalText(body.DisplayName); err != nil {
		writeError(w, err)
		return
	}

	request, err := commandRequest(w, r, &body)
	if err != nil {
		writeError(w, err)
		return
	}
	input := model.PatchEmployeeInput{DisplayName: body.DisplayName, IsActive: body.IsActive}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.PatchEmployee(ctx, request, r.PathValue("employee_id"), input)
	writeCommand(w, r, result, err)
}
