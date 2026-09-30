package handler

import (
	"context"
	"net/http"
)

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/employees", s.createEmployee)
	mux.HandleFunc("GET /api/employees", s.read(s.listEmployees))
	mux.HandleFunc("GET /api/employees/{employee_id}", s.read(s.getEmployee))
	mux.HandleFunc("PATCH /api/employees/{employee_id}", s.patchEmployee)
	mux.HandleFunc("POST /api/credential-resolutions", s.resolveCredential)
	mux.HandleFunc("GET /api/employees/{employee_id}/credentials", s.read(s.listCredentials))
	mux.HandleFunc("POST /api/employees/{employee_id}/credentials", s.createCredential)
	mux.HandleFunc("PATCH /api/employees/{employee_id}/credentials/{credential_id}", s.disableCredential)

	mux.HandleFunc("GET /api/batteries", s.read(s.listBatteries))
	mux.HandleFunc("POST /api/batteries", s.registerBattery)
	mux.HandleFunc("GET /api/batteries/{battery_id}", s.read(s.getBattery))
	mux.HandleFunc("POST /api/batteries/{battery_id}/take", s.takeBattery)
	mux.HandleFunc("POST /api/batteries/{battery_id}/return", s.returnBattery)
	mux.HandleFunc("POST /api/batteries/{battery_id}/move", s.moveBattery)

	mux.HandleFunc("GET /api/batteries/{battery_id}/operations", s.read(s.listBatteryOperations))
	mux.HandleFunc("GET /api/employees/{employee_id}/batteries", s.read(s.listCustody))
	mux.HandleFunc("GET /api/employees/{employee_id}/operations", s.read(s.listEmployeeOperations))
	mux.HandleFunc("GET /api/operations", s.read(s.listOperations))
	mux.HandleFunc("GET /api/operations/{operation_id}", s.read(s.getOperation))
}

func (s *Server) getEmployee(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return s.service.GetEmployeeByID(ctx, r.PathValue("employee_id"))
}

func (s *Server) getBattery(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return s.service.GetBatteryByID(ctx, r.PathValue("battery_id"))
}

func (s *Server) getOperation(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return s.service.GetOperationByID(ctx, r.PathValue("operation_id"))
}
