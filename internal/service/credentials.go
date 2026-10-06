package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

func (s *Service) CreateCredential(ctx context.Context, employeeID int64, input model.CreateCredentialInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		if _, err := tx.LockEmployee(ctx, employeeID); err != nil {
			return 0, nil, err
		}
		if err := tx.LockCredentialValue(ctx, input.Value); err != nil {
			return 0, nil, err
		}
		active, usedByAnotherEmployee, err := tx.CredentialUsage(ctx, input.Value, employeeID)
		if err != nil {
			return 0, nil, err
		}
		if active {
			return 0, nil, model.Conflict("ACTIVE_CREDENTIAL_EXISTS")
		}
		if usedByAnotherEmployee {
			return 0, nil, model.Conflict("CREDENTIAL_REUSE_UNCONFIRMED")
		}
		if input.ReplacesCredentialID != nil {
			if err := revokeCredential(ctx, tx, employeeID, *input.ReplacesCredentialID); err != nil {
				return 0, nil, err
			}
		}
		// Only replacing the current card can free the employee's active slot.
		hasActiveCard, err := tx.HasActiveCredential(ctx, employeeID)
		if err != nil {
			return 0, nil, err
		}
		if hasActiveCard {
			return 0, nil, model.Conflict("EMPLOYEE_ACTIVE_CREDENTIAL_EXISTS")
		}
		credentialID, err := tx.InsertCredential(ctx, employeeID, input.Value)
		if err != nil {
			return 0, nil, err
		}
		credential, err := tx.GetCredential(ctx, employeeID, credentialID)
		return 201, credential, err
	})
}

func (s *Service) DisableCredential(ctx context.Context, employeeID, credentialID int64, active bool) (model.CommandResponse, error) {
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		if active {
			return 0, nil, model.NewError(422, "VALIDATION_FAILED", "Карту можно только отключить")
		}
		if _, err := tx.LockEmployee(ctx, employeeID); err != nil {
			return 0, nil, err
		}
		if err := revokeCredential(ctx, tx, employeeID, credentialID); err != nil {
			return 0, nil, err
		}
		credential, err := tx.GetCredential(ctx, employeeID, credentialID)
		return 200, credential, err
	})
}

func revokeCredential(ctx context.Context, tx *repository.Tx, employeeID, credentialID int64) error {
	if _, err := tx.GetCredential(ctx, employeeID, credentialID); err != nil {
		return err
	}
	return tx.DisableCredential(ctx, employeeID, credentialID)
}
