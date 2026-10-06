package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
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
	if err := decodeRequest(w, r, &body, "display_name personnel_number"); err != nil {
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

	input := model.CreateEmployeeInput{DisplayName: *body.DisplayName, PersonnelNumber: body.PersonnelNumber}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.CreateEmployee(ctx, input)
	})
}

func (s *Server) patchEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID, err := pathID(r, "employee_id")
	if err != nil {
		writeError(w, err)
		return
	}
	var body patchEmployeeRequest
	if err := decodeRequest(w, r, &body, "display_name is_active"); err != nil {
		writeError(w, err)
		return
	}
	if err := optionalText(body.DisplayName); err != nil {
		writeError(w, err)
		return
	}

	input := model.PatchEmployeeInput{DisplayName: body.DisplayName, IsActive: body.IsActive}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.PatchEmployee(ctx, employeeID, input)
	})
}
