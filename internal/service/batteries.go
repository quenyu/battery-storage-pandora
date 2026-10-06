package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

// Battery commands lock in this order: employee, credential, battery, target location.
// The latest operation is read only after the battery lock, so it is current.

func (s *Service) RegisterBattery(ctx context.Context, input model.RegisterBatteryInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		_, credential, err := commandActor(ctx, tx, input.ActorCredential)
		if err != nil {
			return 0, nil, err
		}
		batteryID, inserted, err := tx.InsertBattery(ctx, input.InventoryCode, input.SerialNumber)
		if err != nil {
			return 0, nil, err
		}
		if !inserted {
			return replayStore(ctx, tx, input, credential.ID)
		}
		return recordOperation(ctx, tx, batteryID, model.OperationStore, credential.ID, &input.DestinationLocation)
	})
}

func replayStore(ctx context.Context, tx *repository.Tx, input model.RegisterBatteryInput, credentialID int64) (int, any, error) {
	batteryID, err := tx.LockBattery(ctx, input.InventoryCode)
	if err != nil {
		return 0, nil, err
	}
	battery, last, err := currentState(ctx, tx, batteryID)
	if err != nil {
		return 0, nil, err
	}
	if isReplay(last, model.OperationStore, credentialID, &input.DestinationLocation) &&
		equalText(battery.SerialNumber, input.SerialNumber) {
		return 200, model.CommandResult{Battery: battery, Operation: last}, nil
	}
	return 0, nil, model.Conflict("INVENTORY_CODE_EXISTS")
}

func (s *Service) TakeBattery(ctx context.Context, inventoryCode string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, inventoryCode, model.OperationTake, input.ActorCredential, nil)
}

func (s *Service) ReturnBattery(ctx context.Context, inventoryCode string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, inventoryCode, model.OperationReturn, input.ActorCredential, &input.DestinationLocation)
}

func (s *Service) MoveBattery(ctx context.Context, inventoryCode string, input model.BatteryCommandInput) (model.CommandResponse, error) {
	return s.changeBattery(ctx, inventoryCode, model.OperationMove, input.ActorCredential, &input.DestinationLocation)
}

func (s *Service) changeBattery(ctx context.Context, inventoryCode, kind, credentialValue string, destination *string) (model.CommandResponse, error) {
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		employee, credential, err := commandActor(ctx, tx, credentialValue)
		if err != nil {
			return 0, nil, err
		}
		batteryID, err := tx.LockBattery(ctx, inventoryCode)
		if err != nil {
			return 0, nil, err
		}
		battery, last, err := currentState(ctx, tx, batteryID)
		if err != nil {
			return 0, nil, err
		}
		// A repeated latest command is answered before state checks, so a retried
		// TAKE or MOVE does not turn into BATTERY_ALREADY_ISSUED or SAME_LOCATION.
		if isReplay(last, kind, credential.ID, destination) {
			return 200, model.CommandResult{Battery: battery, Operation: last}, nil
		}
		if err := checkTransition(battery, kind, employee.ID, destination); err != nil {
			return 0, nil, err
		}
		return recordOperation(ctx, tx, batteryID, kind, credential.ID, destination)
	})
}

func currentState(ctx context.Context, tx *repository.Tx, batteryID int64) (model.Battery, model.Operation, error) {
	battery, err := tx.GetBattery(ctx, batteryID)
	if err != nil {
		return model.Battery{}, model.Operation{}, err
	}
	last, err := tx.LatestOperation(ctx, batteryID)
	return battery, last, err
}

// isReplay reports whether the command repeats the latest operation exactly.
func isReplay(last model.Operation, kind string, credentialID int64, destination *string) bool {
	return last.Type == kind && last.CredentialID == credentialID && equalText(last.DestinationLocation, destination)
}

func checkTransition(battery model.Battery, kind string, actorID int64, destination *string) error {
	switch kind {
	case model.OperationTake:
		if battery.Status != model.BatteryStored {
			return model.Conflict("BATTERY_ALREADY_ISSUED")
		}
	case model.OperationReturn:
		if battery.Status != model.BatteryIssued {
			return model.Conflict("INVALID_BATTERY_STATE")
		}
		if *battery.CurrentHolderEmployeeID != actorID {
			return model.NewError(403, "RETURN_NOT_ALLOWED", "Вернуть АКБ может только взявший сотрудник")
		}
	case model.OperationMove:
		if battery.Status != model.BatteryStored {
			return model.Conflict("INVALID_BATTERY_STATE")
		}
		if *battery.CurrentLocation == *destination {
			return model.Conflict("SAME_LOCATION")
		}
	default:
		return model.NewError(500, "INTERNAL_ERROR", "Неизвестная команда")
	}
	return nil
}

func recordOperation(ctx context.Context, tx *repository.Tx, batteryID int64, kind string, credentialID int64, destination *string) (int, any, error) {
	if destination != nil {
		if err := tx.LockLocation(ctx, *destination); err != nil {
			return 0, nil, err
		}
		occupied, err := tx.LocationOccupied(ctx, *destination)
		if err != nil {
			return 0, nil, err
		}
		if occupied {
			return 0, nil, model.Conflict("LOCATION_OCCUPIED")
		}
	}
	if err := tx.InsertOperation(ctx, batteryID, kind, credentialID, destination); err != nil {
		return 0, nil, err
	}
	battery, operation, err := currentState(ctx, tx, batteryID)
	if err != nil {
		return 0, nil, err
	}
	return 201, model.CommandResult{Battery: battery, Operation: operation}, nil
}

func equalText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
