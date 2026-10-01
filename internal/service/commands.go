package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
	"encoding/json"
)

type command func(*repository.Tx) (any, error)

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
		return replayCommand(ctx, tx, request)
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
		// Save the business error without retaining any partial changes.
		if err := tx.RollbackToSavepoint(ctx); err != nil {
			return model.CommandResponse{}, err
		}
		status = commandError.Status
		value = model.ErrorBody(commandError, request.RequestID)
	}
	return commitCommandResponse(ctx, tx, request.Key, status, value)
}

func replayCommand(ctx context.Context, tx *repository.Tx, request model.CommandRequest) (model.CommandResponse, error) {
	saved, err := tx.SavedRequest(ctx, request.Key)
	if err != nil {
		return model.CommandResponse{}, err
	}
	if saved.Hash != request.Hash {
		return model.CommandResponse{}, model.Conflict("IDEMPOTENCY_KEY_REUSED")
	}
	return model.CommandResponse{Status: saved.Status, Body: saved.Body, Replayed: true}, nil
}

func commitCommandResponse(ctx context.Context, tx *repository.Tx, requestKey string, status int, value any) (model.CommandResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return model.CommandResponse{}, err
	}
	if err := tx.SaveResponse(ctx, requestKey, status, body); err != nil {
		return model.CommandResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.CommandResponse{}, model.NewError(503, "TEMPORARILY_UNAVAILABLE", "Повторите тот же ключ")
	}
	return model.CommandResponse{Status: status, Body: body}, nil
}
