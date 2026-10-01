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
			Status:          model.BatteryStored,
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
			Type:                model.OperationStore,
			ActorEmployeeID:     employee.ID,
			CredentialID:        credential.ID,
			DestinationStatus:   model.BatteryStored,
			DestinationLocation: &input.DestinationLocation,
		}
		return finishBatteryCommand(ctx, tx, request.Key, operation)
	})
}

func (s *Service) TakeBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, request, batteryID, input, model.OperationTake)
}

func (s *Service) ReturnBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, request, batteryID, input, model.OperationReturn)
}

func (s *Service) MoveBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, request, batteryID, input, model.OperationMove)
}

func (s *Service) changeBattery(ctx context.Context, request model.CommandRequest, batteryID string, input model.BatteryCommandInput, kind string) (model.CommandResponse, error) {
	return s.runCommand(ctx, request, 201, func(tx *repository.Tx) (any, error) {
		// Acquire locks in order: employee, credential, then battery.
		employee, credential, err := commandActor(ctx, tx, input.ActorCredential)
		if err != nil {
			return nil, err
		}
		battery, err := tx.LockBattery(ctx, batteryID)
		if err != nil {
			return nil, err
		}
		if err := checkBatteryObservation(battery, input); err != nil {
			return nil, err
		}
		operation := prepareBatteryOperation(battery, employee.ID, credential.ID, kind)
		if err := setBatteryDestination(&operation, battery, input); err != nil {
			return nil, err
		}
		if err := tx.UpdateBatteryState(ctx, operation); err != nil {
			return nil, err
		}
		return finishBatteryCommand(ctx, tx, request.Key, operation)
	})
}

func checkBatteryObservation(battery model.Battery, input model.BatteryCommandInput) error {
	// A stale version takes precedence when both observations disagree.
	if input.ExpectedVersion != nil && *input.ExpectedVersion != battery.Version {
		return model.Conflict("STATE_VERSION_MISMATCH")
	}
	if input.ObservedSourceLocation != nil &&
		(battery.CurrentLocation == nil || *battery.CurrentLocation != *input.ObservedSourceLocation) {
		return model.Conflict("SOURCE_MISMATCH")
	}
	return nil
}

func prepareBatteryOperation(battery model.Battery, employeeID, credentialID, kind string) model.Operation {
	return model.Operation{
		ID:                     newID(),
		BatteryID:              battery.ID,
		BatteryVersion:         battery.Version + 1,
		Type:                   kind,
		ActorEmployeeID:        employeeID,
		CredentialID:           credentialID,
		SourceStatus:           &battery.Status,
		SourceLocation:         battery.CurrentLocation,
		SourceHolderEmployeeID: battery.CurrentHolderEmployeeID,
	}
}

func setBatteryDestination(operation *model.Operation, battery model.Battery, input model.BatteryCommandInput) error {
	switch operation.Type {
	case model.OperationTake:
		if battery.Status == model.BatteryIssued {
			return model.Conflict("BATTERY_ALREADY_ISSUED")
		}
		if battery.Status != model.BatteryStored {
			return model.Conflict("INVALID_BATTERY_STATE")
		}
		operation.DestinationStatus = model.BatteryIssued
		operation.DestinationHolderEmployeeID = &operation.ActorEmployeeID
	case model.OperationReturn:
		if battery.Status != model.BatteryIssued {
			return model.Conflict("INVALID_BATTERY_STATE")
		}
		if battery.CurrentHolderEmployeeID == nil || *battery.CurrentHolderEmployeeID != operation.ActorEmployeeID {
			return model.NewError(403, "RETURN_NOT_ALLOWED", "Вернуть АКБ может только взявший сотрудник")
		}
		operation.DestinationStatus = model.BatteryStored
		operation.DestinationLocation = &input.DestinationLocation
	case model.OperationMove:
		if battery.Status != model.BatteryStored {
			return model.Conflict("INVALID_BATTERY_STATE")
		}
		if battery.CurrentLocation != nil && *battery.CurrentLocation == input.DestinationLocation {
			return model.Conflict("SAME_LOCATION")
		}
		operation.DestinationStatus = model.BatteryStored
		operation.DestinationLocation = &input.DestinationLocation
	default:
		return model.NewError(500, "INTERNAL_ERROR", "Неизвестная команда")
	}
	return nil
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
