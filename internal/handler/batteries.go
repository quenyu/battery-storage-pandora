package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
	"time"
)

type registerBatteryRequest struct {
	InventoryCode       *string `json:"inventory_code"`
	SerialNumber        *string `json:"serial_number"`
	ActorCredential     *string `json:"actor_credential_value"`
	DestinationLocation *string `json:"destination_location"`
}

type takeBatteryRequest struct {
	ActorCredential        *string `json:"actor_credential_value"`
	ExpectedVersion        *int64  `json:"expected_version"`
	ObservedSourceLocation *string `json:"observed_source_location"`
}

type returnBatteryRequest struct {
	ActorCredential     *string `json:"actor_credential_value"`
	DestinationLocation *string `json:"destination_location"`
	ExpectedVersion     *int64  `json:"expected_version"`
}

type moveBatteryRequest struct {
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

	request, err := commandRequest(w, r, &body)
	if err != nil {
		writeError(w, err)
		return
	}
	input := model.RegisterBatteryInput{
		InventoryCode:       *body.InventoryCode,
		SerialNumber:        body.SerialNumber,
		ActorCredential:     *body.ActorCredential,
		DestinationLocation: *body.DestinationLocation,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.RegisterBattery(ctx, request, input)
	writeCommand(w, r, result, err)
}

func (s *Server) takeBattery(w http.ResponseWriter, r *http.Request) {
	var body takeBatteryRequest
	if err := decodeCommand(w, r, &body, "actor_credential_value expected_version observed_source_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.ActorCredential); err != nil {
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

	request, err := commandRequest(w, r, &body)
	if err != nil {
		writeError(w, err)
		return
	}
	input := model.BatteryCommandInput{
		ActorCredential:        *body.ActorCredential,
		ExpectedVersion:        body.ExpectedVersion,
		ObservedSourceLocation: body.ObservedSourceLocation,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.TakeBattery(ctx, request, r.PathValue("battery_id"), input)
	writeCommand(w, r, result, err)
}

func (s *Server) returnBattery(w http.ResponseWriter, r *http.Request) {
	var body returnBatteryRequest
	if err := decodeCommand(w, r, &body, "actor_credential_value destination_location expected_version"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.ActorCredential, body.DestinationLocation); err != nil {
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

	request, err := commandRequest(w, r, &body)
	if err != nil {
		writeError(w, err)
		return
	}
	input := model.BatteryCommandInput{
		ActorCredential:     *body.ActorCredential,
		DestinationLocation: *body.DestinationLocation,
		ExpectedVersion:     body.ExpectedVersion,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.ReturnBattery(ctx, request, r.PathValue("battery_id"), input)
	writeCommand(w, r, result, err)
}

func (s *Server) moveBattery(w http.ResponseWriter, r *http.Request) {
	var body moveBatteryRequest
	if err := decodeCommand(w, r, &body, "actor_credential_value destination_location expected_version observed_source_location"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.ActorCredential, body.DestinationLocation); err != nil {
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

	request, err := commandRequest(w, r, &body)
	if err != nil {
		writeError(w, err)
		return
	}
	input := model.BatteryCommandInput{
		ActorCredential:        *body.ActorCredential,
		DestinationLocation:    *body.DestinationLocation,
		ExpectedVersion:        body.ExpectedVersion,
		ObservedSourceLocation: body.ObservedSourceLocation,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := s.service.MoveBattery(ctx, request, r.PathValue("battery_id"), input)
	writeCommand(w, r, result, err)
}
