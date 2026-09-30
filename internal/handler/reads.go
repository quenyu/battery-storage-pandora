package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

func listResponse(items any, err error) (any, error) {
	return model.ListResult{Items: items}, err
}

func (s *Server) listEmployees(ctx context.Context, r *http.Request) (any, error) {
	filters, err := parseFilters(r, "is_active")
	if err != nil {
		return nil, err
	}
	return listResponse(s.service.ListEmployees(ctx, filters))
}

func (s *Server) listCredentials(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return listResponse(s.service.ListCredentials(ctx, r.PathValue("employee_id")))
}

func (s *Server) listBatteries(ctx context.Context, r *http.Request) (any, error) {
	filters, err := parseFilters(r, "inventory_code status holder_employee_id location")
	if err != nil {
		return nil, err
	}
	return listResponse(s.service.ListBatteries(ctx, filters))
}

func (s *Server) listCustody(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return listResponse(s.service.ListCustody(ctx, r.PathValue("employee_id")))
}

func (s *Server) listBatteryOperations(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return listResponse(s.service.ListBatteryOperations(ctx, r.PathValue("battery_id")))
}

func (s *Server) listEmployeeOperations(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return listResponse(s.service.ListEmployeeOperations(ctx, r.PathValue("employee_id")))
}

func (s *Server) listOperations(ctx context.Context, r *http.Request) (any, error) {
	filters, err := parseFilters(r, "battery_id employee_id location device_code type from to")
	if err != nil {
		return nil, err
	}
	return listResponse(s.service.ListOperations(ctx, filters))
}
