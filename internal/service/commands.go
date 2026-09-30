package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
	"encoding/json"
)

type command func(*repository.Tx) (any, error)

// Every command stores its response in the same transaction as its changes.
func (s *Service) runCommand(ctx context.Context, request model.CommandRequest, status int, execute command) (model.CommandResponse, error) {
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return model.CommandResponse{}, err
	}
	defer tx.Rollback()
	reserved, err := tx.ReserveRequest(ctx, request.Key, request.Hash)
	if err != nil {
		return model.CommandResponse{}, err
	}
	if !reserved {
		saved, err := tx.SavedRequest(ctx, request.Key)
		if err != nil {
			return model.CommandResponse{}, err
		}
		if saved.Hash != request.Hash {
			return model.CommandResponse{}, model.Conflict("IDEMPOTENCY_KEY_REUSED")
		}
		return model.CommandResponse{Status: saved.Status, Body: saved.Body, Replayed: true}, nil
	}
	if err := tx.Savepoint(ctx); err != nil {
		return model.CommandResponse{}, err
	}
	value, businessError := execute(tx)
	if businessError != nil {
		commandError := ClassifyError(businessError)
		if commandError.Status != 404 && commandError.Status != 409 && commandError.Status != 422 {
			return model.CommandResponse{}, commandError
		}
		// Retain a business conflict result while rolling back its changes.
		if err := tx.RollbackToSavepoint(ctx); err != nil {
			return model.CommandResponse{}, err
		}
		status = commandError.Status
		value = model.ErrorBody(commandError, request.RequestID)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return model.CommandResponse{}, err
	}
	if err := tx.SaveResponse(ctx, request.Key, status, body); err != nil {
		return model.CommandResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.CommandResponse{}, model.NewError(503, "TEMPORARILY_UNAVAILABLE", "Результат фиксации неизвестен; повторите тот же ключ")
	}
	return model.CommandResponse{Status: status, Body: body}, nil
}
