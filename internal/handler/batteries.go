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
	InventoryCode          *string `json:"inventory_code"`
	ActorCredential        *string `json:"actor_credential_value"`
	ExpectedVersion        *int64  `json:"expected_version"`
	ObservedSourceLocation *string `json:"observed_source_location"`
}

type returnBatteryRequest struct {
	InventoryCode       *string `json:"inventory_code"`
	ActorCredential     *string `json:"actor_credential_value"`
	DestinationLocation *string `json:"destination_location"`
	ExpectedVersion     *int64  `json:"expected_version"`
}

type moveBatteryRequest struct {
	InventoryCode          *string `json:"inventory_code"`
	ActorCredential        *string `json:"actor_credential_value"`
	DestinationLocation    *string `json:"destination_location"`
	ExpectedVersion        *int64  `json:"expected_version"`
	ObservedSourceLocation *string `json:"observed_source_location"`
}

func (s *Server) registerBattery(w http.ResponseWriter, r *http.Request) {
	var body registerBatteryRequest
	if err := decodeCommand(w, r, &body, "inventory_code serial_number actor_credential_value destination_location"); err != nil {
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
	if err := decodeCommand(w, r, &body, "inventory_code actor_credential_value expected_version observed_source_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorCredential); err != nil {
		writeError(w, err)
		return
	}
	if err := validateVersion(body.ExpectedVersion); err != nil {
		writeError(w, err)
		return
	}
	if err := validateLocations(body.ObservedSourceLocation); err != nil {
		writeError(w, err)
		return
	}

	input := model.BatteryCommandInput{
		ActorCredential:        *body.ActorCredential,
		ExpectedVersion:        body.ExpectedVersion,
		ObservedSourceLocation: body.ObservedSourceLocation,
	}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.TakeBattery(ctx, *body.InventoryCode, input)
	})
}

func (s *Server) returnBattery(w http.ResponseWriter, r *http.Request) {
	var body returnBatteryRequest
	if err := decodeCommand(w, r, &body, "inventory_code actor_credential_value destination_location expected_version"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorCredential, body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}
	if err := validateVersion(body.ExpectedVersion); err != nil {
		writeError(w, err)
		return
	}
	if err := validateLocations(body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}

	input := model.BatteryCommandInput{
		ActorCredential:     *body.ActorCredential,
		DestinationLocation: *body.DestinationLocation,
		ExpectedVersion:     body.ExpectedVersion,
	}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.ReturnBattery(ctx, *body.InventoryCode, input)
	})
}

func (s *Server) moveBattery(w http.ResponseWriter, r *http.Request) {
	var body moveBatteryRequest
	if err := decodeCommand(w, r, &body, "inventory_code actor_credential_value destination_location expected_version observed_source_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.InventoryCode, body.ActorCredential, body.DestinationLocation); err != nil {
		writeError(w, err)
		return
	}
	if err := validateVersion(body.ExpectedVersion); err != nil {
		writeError(w, err)
		return
	}
	if err := validateLocations(body.DestinationLocation, body.ObservedSourceLocation); err != nil {
		writeError(w, err)
		return
	}

	input := model.BatteryCommandInput{
		ActorCredential:        *body.ActorCredential,
		DestinationLocation:    *body.DestinationLocation,
		ExpectedVersion:        body.ExpectedVersion,
		ObservedSourceLocation: body.ObservedSourceLocation,
	}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.MoveBattery(ctx, *body.InventoryCode, input)
	})
}
