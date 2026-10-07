package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
	"time"
)

func readContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 15*time.Second)
}

// resourceID validates a read request without query parameters and returns its path ID.
func resourceID(r *http.Request, name string) (int64, error) {
	if err := noQuery(r); err != nil {
		return 0, err
	}
	return pathID(r, name)
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := resourceID(r, "user_id")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := readContext(r)
	defer cancel()
	result, err := s.service.GetUserByID(ctx, id)
	writeReadResult(w, result, err)
}

func (s *Server) getBattery(w http.ResponseWriter, r *http.Request) {
	id, err := resourceID(r, "battery_id")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := readContext(r)
	defer cancel()
	result, err := s.service.GetBatteryByID(ctx, id)
	writeReadResult(w, result, err)
}

func (s *Server) getOperation(w http.ResponseWriter, r *http.Request) {
	id, err := resourceID(r, "operation_id")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := readContext(r)
	defer cancel()
	result, err := s.service.GetOperationByID(ctx, id)
	writeReadResult(w, result, err)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	filters, err := parseFilters(r, "is_active")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := readContext(r)
	defer cancel()
	result, err := s.service.ListUsers(ctx, filters)
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listBatteries(w http.ResponseWriter, r *http.Request) {
	filters, err := parseFilters(r, "inventory_code status holder_user_id location")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := readContext(r)
	defer cancel()
	result, err := s.service.ListBatteries(ctx, filters)
	writeReadResult(w, model.ListResult{Items: result}, err)
}

func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	filters, err := parseFilters(r, "battery_id user_id location type from to")
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := readContext(r)
	defer cancel()
	result, err := s.service.ListOperations(ctx, filters)
	writeReadResult(w, model.ListResult{Items: result}, err)
}
