package handler

import "net/http"

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/users", s.createUser)
	mux.HandleFunc("GET /api/users", s.listUsers)
	mux.HandleFunc("GET /api/users/{user_id}", s.getUser)
	mux.HandleFunc("PATCH /api/users/{user_id}", s.patchUser)

	mux.HandleFunc("POST /api/batteries", s.registerBattery)
	mux.HandleFunc("GET /api/batteries", s.listBatteries)
	mux.HandleFunc("GET /api/batteries/{battery_id}", s.getBattery)
	mux.HandleFunc("POST /api/batteries/take", s.takeBattery)
	mux.HandleFunc("POST /api/batteries/return", s.returnBattery)
	mux.HandleFunc("POST /api/batteries/move", s.moveBattery)
	mux.HandleFunc("GET /api/operations", s.listOperations)
	mux.HandleFunc("GET /api/operations/{operation_id}", s.getOperation)
}
