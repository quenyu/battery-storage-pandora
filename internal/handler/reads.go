package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
	"time"
)

func (s *Server) getEmployee(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	if err := noQuery(r); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.GetEmployeeByID(ctx, r.PathValue("employee_id"))
	writeReadResult(w, result, err)
}

func (s *Server) getBattery(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	if err := noQuery(r); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.GetBatteryByID(ctx, r.PathValue("battery_id"))
	writeReadResult(w, result, err)
}

func (s *Server) getOperation(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	if err := noQuery(r); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.GetOperationByID(ctx, r.PathValue("operation_id"))
	writeReadResult(w, result, err)
}

func (s *Server) listEmployees(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	filters, err := parseFilters(r, "is_active")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListEmployees(ctx, filters)
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	if err := noQuery(r); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListCredentials(ctx, r.PathValue("employee_id"))
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listBatteries(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	filters, err := parseFilters(r, "inventory_code status holder_employee_id location")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListBatteries(ctx, filters)
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listCustody(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	if err := noQuery(r); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListCustody(ctx, r.PathValue("employee_id"))
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listBatteryOperations(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	if err := noQuery(r); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListBatteryOperations(ctx, r.PathValue("battery_id"))
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listEmployeeOperations(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	if err := noQuery(r); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListEmployeeOperations(ctx, r.PathValue("employee_id"))
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	filters, err := parseFilters(r, "battery_id employee_id location type from to")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListOperations(ctx, filters)
	writeReadResult(w, model.ListResult{Items: result}, err)
}
