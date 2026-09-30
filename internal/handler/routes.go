package handler

import (
	"context"
	"net/http"
)

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/employees", s.command("display_name personnel_number?", s.createEmployee))
	mux.HandleFunc("GET /api/employees", s.read(s.listEmployees))
	mux.HandleFunc("GET /api/employees/{employee_id}", s.read(s.getEmployee))
	mux.HandleFunc("PATCH /api/employees/{employee_id}", s.command("display_name? is_active?", s.patchEmployee))
	mux.HandleFunc("POST /api/credential-resolutions", s.resolveCredential)
	mux.HandleFunc("GET /api/employees/{employee_id}/credentials", s.read(s.listCredentials))
	mux.HandleFunc("POST /api/employees/{employee_id}/credentials", s.command("value replaces_credential_id?", s.createCredential))
	mux.HandleFunc("PATCH /api/employees/{employee_id}/credentials/{credential_id}", s.command("is_active", s.disableCredential))

	mux.HandleFunc("GET /api/batteries", s.read(s.listBatteries))
	mux.HandleFunc("POST /api/batteries", s.command("inventory_code serial_number? actor_credential_value destination_location", s.registerBattery))
	mux.HandleFunc("GET /api/batteries/{battery_id}", s.read(s.getBattery))
	mux.HandleFunc("POST /api/batteries/{battery_id}/take", s.command("actor_credential_value expected_version? observed_source_location?", s.takeBattery))
	mux.HandleFunc("POST /api/batteries/{battery_id}/return", s.command("actor_credential_value destination_location expected_version?", s.returnBattery))
	mux.HandleFunc("POST /api/batteries/{battery_id}/move", s.command("actor_credential_value destination_location expected_version? observed_source_location?", s.moveBattery))

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
	return s.service.GetEmployee(ctx, r.PathValue("employee_id"))
}

func (s *Server) getBattery(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return s.service.GetBattery(ctx, r.PathValue("battery_id"))
}

func (s *Server) getOperation(ctx context.Context, r *http.Request) (any, error) {
	if err := noQuery(r); err != nil {
		return nil, err
	}
	return s.service.GetOperation(ctx, r.PathValue("operation_id"))
}
