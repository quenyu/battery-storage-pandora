package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

type registerBatteryRequest struct {
	InventoryCode       *string `json:"inventory_code"`
	SerialNumber        *string `json:"serial_number"`
	ActorCredential     *string `json:"actor_credential_value"`
	DestinationLocation *string `json:"destination_location"`
}

type takeBatteryRequest struct {
	InventoryCode   *string `json:"inventory_code"`
	ActorCredential *string `json:"actor_credential_value"`
}

// placeBatteryRequest is the body of RETURN and MOVE.
type placeBatteryRequest struct {
	InventoryCode       *string `json:"inventory_code"`
	ActorCredential     *string `json:"actor_credential_value"`
	DestinationLocation *string `json:"destination_location"`
}

func (s *Server) registerBattery(w http.ResponseWriter, r *http.Request) {
	var body registerBatteryRequest
	if err := decodeRequest(w, r, &body, "inventory_code serial_number actor_credential_value destination_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorCredential, body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}
	if err := optionalText(body.SerialNumber); err != nil {
		writeError(w, err)
		return
	}
	if err := validateLocations(body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}

	input := model.RegisterBatteryInput{
		InventoryCode:       *body.InventoryCode,
		SerialNumber:        body.SerialNumber,
		ActorCredential:     *body.ActorCredential,
		DestinationLocation: *body.DestinationLocation,
	}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.RegisterBattery(ctx, input)
	})
}

func (s *Server) takeBattery(w http.ResponseWriter, r *http.Request) {
	var body takeBatteryRequest
	if err := decodeRequest(w, r, &body, "inventory_code actor_credential_value"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorCredential); err != nil {
		writeError(w, err)
		return
	}

	input := model.BatteryCommandInput{ActorCredential: *body.ActorCredential}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.TakeBattery(ctx, *body.InventoryCode, input)
	})
}

func (s *Server) returnBattery(w http.ResponseWriter, r *http.Request) {
	s.placeBattery(w, r, s.service.ReturnBattery)
}

func (s *Server) moveBattery(w http.ResponseWriter, r *http.Request) {
	s.placeBattery(w, r, s.service.MoveBattery)
}

func (s *Server) placeBattery(w http.ResponseWriter, r *http.Request,
	execute func(context.Context, string, model.BatteryCommandInput) (model.CommandResponse, error)) {
	var body placeBatteryRequest
	if err := decodeRequest(w, r, &body, "inventory_code actor_credential_value destination_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorCredential, body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}
	if err := validateLocations(body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}

	input := model.BatteryCommandInput{ActorCredential: *body.ActorCredential, DestinationLocation: *body.DestinationLocation}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return execute(ctx, *body.InventoryCode, input)
	})
}
