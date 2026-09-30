package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

func (s *Server) createEmployee(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.CreateEmployee(ctx, request, model.CreateEmployeeInput{
		DisplayName:     values.str("display_name"),
		PersonnelNumber: values.optionalString("personnel_number"),
	})
}

func (s *Server) patchEmployee(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.PatchEmployee(ctx, request, r.PathValue("employee_id"), model.PatchEmployeeInput{
		DisplayName: values.optionalString("display_name"),
		IsActive:    values.optionalBool("is_active"),
	})
}
