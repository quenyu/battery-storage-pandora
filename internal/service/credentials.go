package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

func (s *Service) CreateCredential(ctx context.Context, request model.CommandRequest, employeeID string, input model.CreateCredentialInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, request, 201, func(tx *repository.Tx) (any, error) {
		if _, err := tx.LockEmployee(ctx, employeeID); err != nil {
			return nil, err
		}
		if err := tx.LockCredentialValue(ctx, input.Value); err != nil {
			return nil, err
		}
		active, usedByAnotherEmployee, err := tx.CredentialUsage(ctx, input.Value, employeeID)
		if err != nil {
			return nil, err
		}
		if active {
			return nil, model.Conflict("ACTIVE_CREDENTIAL_EXISTS")
		}
		if usedByAnotherEmployee {
			return nil, model.Conflict("CREDENTIAL_REUSE_UNCONFIRMED")
		}
		if input.ReplacesCredentialID != nil {
			if err := revokeCredential(ctx, tx, employeeID, *input.ReplacesCredentialID); err != nil {
				return nil, err
			}
		}
		credentialID := newID()
		if err := tx.InsertCredential(ctx, credentialID, employeeID, input.Value); err != nil {
			return nil, err
		}
		return tx.GetCredential(ctx, employeeID, credentialID)
	})
}

func (s *Service) DisableCredential(ctx context.Context, request model.CommandRequest, employeeID, credentialID string, active bool) (model.CommandResponse, error) {
	return s.runCommand(ctx, request, 200, func(tx *repository.Tx) (any, error) {
		if active {
			return nil, model.NewError(422, "VALIDATION_FAILED", "Карту можно только отключить")
		}
		if _, err := tx.LockEmployee(ctx, employeeID); err != nil {
			return nil, err
		}
		if err := revokeCredential(ctx, tx, employeeID, credentialID); err != nil {
			return nil, err
		}
		return tx.GetCredential(ctx, employeeID, credentialID)
	})
}

func revokeCredential(ctx context.Context, tx *repository.Tx, employeeID, credentialID string) error {
	if _, err := tx.GetCredential(ctx, employeeID, credentialID); err != nil {
		return err
	}
	return tx.DisableCredential(ctx, employeeID, credentialID)
}
