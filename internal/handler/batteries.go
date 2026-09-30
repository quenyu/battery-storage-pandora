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

func (body *registerBatteryRequest) validate() error {
	if err := requiredText(body.InventoryCode, body.ActorCredential, body.DestinationLocation); err != nil {
		return err
	}
	if err := optionalText(body.SerialNumber); err != nil {
		return err
	}
	return validateLocations(body.DestinationLocation)
}

func (s *Server) registerBattery(w http.ResponseWriter, r *http.Request) {
	var body registerBatteryRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	input := model.RegisterBatteryInput{
		InventoryCode: *body.InventoryCode, SerialNumber: body.SerialNumber,
		ActorCredential: *body.ActorCredential, DestinationLocation: *body.DestinationLocation,
	}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.RegisterBattery(ctx, request, input)
	})
}

type takeBatteryRequest struct {
	ActorCredential        *string `json:"actor_credential_value"`
	ExpectedVersion        *int64  `json:"expected_version"`
	ObservedSourceLocation *string `json:"observed_source_location"`
}

func (body *takeBatteryRequest) validate() error {
	if err := requiredText(body.ActorCredential); err != nil {
		return err
	}
	if err := validateVersion(body.ExpectedVersion); err != nil {
		return err
	}
	return validateLocations(body.ObservedSourceLocation)
}

func (s *Server) takeBattery(w http.ResponseWriter, r *http.Request) {
	var body takeBatteryRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	input := model.BatteryCommandInput{
		ActorCredential: *body.ActorCredential, ExpectedVersion: body.ExpectedVersion,
		ObservedSourceLocation: body.ObservedSourceLocation,
	}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.TakeBattery(ctx, request, r.PathValue("battery_id"), input)
	})
}

type returnBatteryRequest struct {
	ActorCredential     *string `json:"actor_credential_value"`
	DestinationLocation *string `json:"destination_location"`
	ExpectedVersion     *int64  `json:"expected_version"`
}

func (body *returnBatteryRequest) validate() error {
	if err := requiredText(body.ActorCredential, body.DestinationLocation); err != nil {
		return err
	}
	if err := validateVersion(body.ExpectedVersion); err != nil {
		return err
	}
	return validateLocations(body.DestinationLocation)
}

func (s *Server) returnBattery(w http.ResponseWriter, r *http.Request) {
	var body returnBatteryRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	input := model.BatteryCommandInput{
		ActorCredential: *body.ActorCredential, DestinationLocation: *body.DestinationLocation,
		ExpectedVersion: body.ExpectedVersion,
	}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.ReturnBattery(ctx, request, r.PathValue("battery_id"), input)
	})
}

type moveBatteryRequest struct {
	ActorCredential        *string `json:"actor_credential_value"`
	DestinationLocation    *string `json:"destination_location"`
	ExpectedVersion        *int64  `json:"expected_version"`
	ObservedSourceLocation *string `json:"observed_source_location"`
}

func (body *moveBatteryRequest) validate() error {
	if err := requiredText(body.ActorCredential, body.DestinationLocation); err != nil {
		return err
	}
	if err := validateVersion(body.ExpectedVersion); err != nil {
		return err
	}
	return validateLocations(body.DestinationLocation, body.ObservedSourceLocation)
}

func (s *Server) moveBattery(w http.ResponseWriter, r *http.Request) {
	var body moveBatteryRequest
	if err := decodeCommand(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	input := model.BatteryCommandInput{
		ActorCredential: *body.ActorCredential, DestinationLocation: *body.DestinationLocation,
		ExpectedVersion: body.ExpectedVersion, ObservedSourceLocation: body.ObservedSourceLocation,
	}
	s.executeCommand(w, r, &body, func(ctx context.Context, request model.CommandRequest) (model.CommandResponse, error) {
		return s.service.MoveBattery(ctx, request, r.PathValue("battery_id"), input)
	})
}
