package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

func (s *Service) RegisterBattery(ctx context.Context, request model.CommandRequest, input model.RegisterBatteryInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, request, 201, func(tx *repository.Tx) (any, error) {
		employee, credential, err := commandActor(ctx, tx, input.ActorCredential)
		if err != nil {
			return nil, err
		}
		battery := model.Battery{
			ID:              newID(),
			InventoryCode:   input.InventoryCode,
			SerialNumber:    input.SerialNumber,
			Status:          "STORED",
			CurrentLocation: &input.DestinationLocation,
			Version:         1,
		}
		if err := tx.InsertBattery(ctx, battery); err != nil {
			return nil, err
		}
		operation := model.Operation{
			ID:                  newID(),
			BatteryID:           battery.ID,
			BatteryVersion:      1,
			Type:                "STORE",
			ActorEmployeeID:     employee.ID,
			CredentialID:        credential.ID,
			DestinationStatus:   "STORED",
			DestinationLocation: &input.DestinationLocation,
		}
		return finishBatteryCommand(ctx, tx, request.Key, operation)
	})
}

func (s *Service) TakeBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, request, batteryID, input, "TAKE")
}

func (s *Service) ReturnBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, request, batteryID, input, "RETURN")
}

func (s *Service) MoveBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, request, batteryID, input, "MOVE")
}

func (s *Service) changeBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput, kind string) (model.CommandResponse, error) {
	return s.runCommand(ctx, request, 201, func(tx *repository.Tx) (any, error) {
		employee, credential, err := commandActor(ctx, tx, input.ActorCredential)
		if err != nil {
			return nil, err
		}
		battery, err := tx.LockBattery(ctx, batteryID)
		if err != nil {
			return nil, err
		}
		if input.ExpectedVersion != nil && *input.ExpectedVersion != battery.Version {
			return nil, model.Conflict("STATE_VERSION_MISMATCH")
		}
		if input.ObservedSourceLocation != nil {
			if battery.CurrentLocation == nil || *battery.CurrentLocation != *input.ObservedSourceLocation {
				return nil, model.Conflict("SOURCE_MISMATCH")
			}
		}
		operation := model.Operation{
			ID:                     newID(),
			BatteryID:              battery.ID,
			BatteryVersion:         battery.Version + 1,
			Type:                   kind,
			ActorEmployeeID:        employee.ID,
			CredentialID:           credential.ID,
			SourceStatus:           &battery.Status,
			SourceLocation:         battery.CurrentLocation,
			SourceHolderEmployeeID: battery.CurrentHolderEmployeeID,
		}
		switch kind {
		case "TAKE":
			if battery.Status == "ISSUED" {
				return nil, model.Conflict("BATTERY_ALREADY_ISSUED")
			}
			if battery.Status != "STORED" {
				return nil, model.Conflict("INVALID_BATTERY_STATE")
			}
			operation.DestinationStatus = "ISSUED"
			operation.DestinationHolderEmployeeID = &employee.ID
		case "RETURN":
			if battery.Status != "ISSUED" {
				return nil, model.Conflict("INVALID_BATTERY_STATE")
			}
			if battery.CurrentHolderEmployeeID == nil || *battery.CurrentHolderEmployeeID != employee.ID {
				return nil, model.NewError(403, "RETURN_NOT_ALLOWED", "Вернуть АКБ может только взявший сотрудник")
			}
			operation.DestinationStatus = "STORED"
			operation.DestinationLocation = &input.DestinationLocation
		case "MOVE":
			if battery.Status != "STORED" {
				return nil, model.Conflict("INVALID_BATTERY_STATE")
			}
			if battery.CurrentLocation != nil && *battery.CurrentLocation == input.DestinationLocation {
				return nil, model.Conflict("SAME_LOCATION")
			}
			operation.DestinationStatus = "STORED"
			operation.DestinationLocation = &input.DestinationLocation
		default:
			return nil, model.NewError(500, "INTERNAL_ERROR", "Неизвестная команда")
		}
		if err := tx.UpdateBatteryState(ctx, operation); err != nil {
			return nil, err
		}
		return finishBatteryCommand(ctx, tx, request.Key, operation)
	})
}

func finishBatteryCommand(ctx context.Context, tx *repository.Tx, requestKey string, operation model.Operation) (model.CommandResult, error) {
	if err := tx.RecordOperation(ctx, &operation, requestKey); err != nil {
		return model.CommandResult{}, err
	}
	current, err := tx.GetBattery(ctx, operation.BatteryID)
	if err != nil {
		return model.CommandResult{}, err
	}
	return model.CommandResult{Battery: current, Operation: operation}, nil
}
