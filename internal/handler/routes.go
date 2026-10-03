package handler

import "net/http"

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/employees", s.createEmployee)
	mux.HandleFunc("GET /api/employees", s.listEmployees)
	mux.HandleFunc("GET /api/employees/{employee_id}", s.getEmployee)
	mux.HandleFunc("PATCH /api/employees/{employee_id}", s.patchEmployee)
	mux.HandleFunc("POST /api/credential-resolutions", s.resolveCredential)
	mux.HandleFunc("GET /api/employees/{employee_id}/credentials", s.listCredentials)
	mux.HandleFunc("POST /api/employees/{employee_id}/credentials", s.createCredential)
	mux.HandleFunc("PATCH /api/employees/{employee_id}/credentials/{credential_id}", s.disableCredential)

	mux.HandleFunc("POST /api/batteries", s.registerBattery)
	mux.HandleFunc("GET /api/batteries", s.listBatteries)
	mux.HandleFunc("GET /api/batteries/{battery_id}", s.getBattery)
	mux.HandleFunc("POST /api/batteries/take", s.takeBattery)
	mux.HandleFunc("POST /api/batteries/return", s.returnBattery)
	mux.HandleFunc("POST /api/batteries/move", s.moveBattery)
	mux.HandleFunc("GET /api/batteries/{battery_id}/operations", s.listBatteryOperations)
	mux.HandleFunc("GET /api/employees/{employee_id}/batteries", s.listCustody)
	mux.HandleFunc("GET /api/employees/{employee_id}/operations", s.listEmployeeOperations)
	mux.HandleFunc("GET /api/operations", s.listOperations)
	mux.HandleFunc("GET /api/operations/{operation_id}", s.getOperation)
}
