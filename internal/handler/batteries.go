package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

type registerBatteryRequest struct {
	InventoryCode       *string `json:"inventory_code"`
	ActorBarcode        *string `json:"actor_barcode"`
	DestinationLocation *string `json:"destination_location"`
}

type takeBatteryRequest struct {
	InventoryCode *string `json:"inventory_code"`
	ActorBarcode  *string `json:"actor_barcode"`
}

// placeBatteryRequest is the body of RETURN and MOVE.
type placeBatteryRequest struct {
	InventoryCode       *string `json:"inventory_code"`
	ActorBarcode        *string `json:"actor_barcode"`
	DestinationLocation *string `json:"destination_location"`
}

func (s *Server) registerBattery(w http.ResponseWriter, r *http.Request) {
	var body registerBatteryRequest
	if err := decodeRequest(w, r, &body, "inventory_code actor_barcode destination_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorBarcode, body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}
	if err := validateLocations(body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}

	input := model.RegisterBatteryInput{
		InventoryCode:       *body.InventoryCode,
		ActorBarcode:        *body.ActorBarcode,
		DestinationLocation: *body.DestinationLocation,
	}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.RegisterBattery(ctx, input)
	})
}

func (s *Server) takeBattery(w http.ResponseWriter, r *http.Request) {
	var body takeBatteryRequest
	if err := decodeRequest(w, r, &body, "inventory_code actor_barcode"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorBarcode); err != nil {
		writeError(w, err)
		return
	}

	input := model.BatteryCommandInput{ActorBarcode: *body.ActorBarcode}
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
	if err := decodeRequest(w, r, &body, "inventory_code actor_barcode destination_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorBarcode, body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}
	if err := validateLocations(body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}

	input := model.BatteryCommandInput{ActorBarcode: *body.ActorBarcode, DestinationLocation: *body.DestinationLocation}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return execute(ctx, *body.InventoryCode, input)
	})
}
