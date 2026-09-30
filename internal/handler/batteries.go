package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

func (s *Server) registerBattery(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.RegisterBattery(ctx, request, model.RegisterBatteryInput{
		InventoryCode:       values.str("inventory_code"),
		SerialNumber:        values.optionalString("serial_number"),
		ActorCredential:     values.str("actor_credential_value"),
		DestinationLocation: values.str("destination_location"),
	})
}

func batteryInput(values input) model.BatteryCommandInput {
	return model.BatteryCommandInput{
		ActorCredential:        values.str("actor_credential_value"),
		DestinationLocation:    values.str("destination_location"),
		ExpectedVersion:        values.optionalInt64("expected_version"),
		ObservedSourceLocation: values.optionalString("observed_source_location"),
	}
}

func (s *Server) takeBattery(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.TakeBattery(ctx, request, r.PathValue("battery_id"), batteryInput(values))
}

func (s *Server) returnBattery(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.ReturnBattery(ctx, request, r.PathValue("battery_id"), batteryInput(values))
}

func (s *Server) moveBattery(ctx context.Context, request model.CommandRequest, r *http.Request, values input) (model.CommandResponse, error) {
	return s.service.MoveBattery(ctx, request, r.PathValue("battery_id"), batteryInput(values))
}
