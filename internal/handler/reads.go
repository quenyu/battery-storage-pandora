package handler

import (
	"context"
	"net/http"
)

func (s *Server) listEmployees(ctx context.Context, r *http.Request) (any, error) {
	options, err := parseFilters(r, "is_active")
	if err != nil {
		return nil, err
	}
	return s.service.ListEmployees(ctx, options)
}

func (s *Server) listCredentials(ctx context.Context, r *http.Request) (any, error) {
	options, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	return s.service.ListCredentials(ctx, r.PathValue("employee_id"), options)
}

func (s *Server) listBatteries(ctx context.Context, r *http.Request) (any, error) {
	options, err := parseFilters(r, "inventory_code status holder_employee_id location")
	if err != nil {
		return nil, err
	}
	return s.service.ListBatteries(ctx, options)
}

func (s *Server) listCustody(ctx context.Context, r *http.Request) (any, error) {
	options, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	return s.service.ListCustody(ctx, r.PathValue("employee_id"), options)
}

func (s *Server) listBatteryOperations(ctx context.Context, r *http.Request) (any, error) {
	options, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	return s.service.ListBatteryOperations(ctx, r.PathValue("battery_id"), options)
}

func (s *Server) listEmployeeOperations(ctx context.Context, r *http.Request) (any, error) {
	options, err := parseFilters(r, "")
	if err != nil {
		return nil, err
	}
	return s.service.ListEmployeeOperations(ctx, r.PathValue("employee_id"), options)
}

func (s *Server) listOperations(ctx context.Context, r *http.Request) (any, error) {
	options, err := parseFilters(r, "battery_id employee_id location device_code type from to")
	if err != nil {
		return nil, err
	}
	return s.service.ListOperations(ctx, options)
}
