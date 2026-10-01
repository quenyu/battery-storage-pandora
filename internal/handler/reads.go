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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ListResult{Items: result})
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ListResult{Items: result})
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ListResult{Items: result})
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ListResult{Items: result})
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ListResult{Items: result})
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
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ListResult{Items: result})
}

func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	if err := normalizePath(r); err != nil {
		writeError(w, err)
		return
	}
	filters, err := parseFilters(r, "battery_id employee_id location device_code type from to")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ListOperations(ctx, filters)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ListResult{Items: result})
}
