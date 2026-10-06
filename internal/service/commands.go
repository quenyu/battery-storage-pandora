package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
	"encoding/json"
)

// command returns the HTTP status of its result: 201 for a new resource or
// operation, 200 for an update or a replayed battery command.
type command func(*repository.Tx) (int, any, error)

func (s *Service) runCommand(ctx context.Context, execute command) (model.CommandResponse, error) {
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return model.CommandResponse{}, err
	}
	defer tx.Rollback()
	status, value, err := execute(tx)
	if err != nil {
		return model.CommandResponse{}, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return model.CommandResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.CommandResponse{}, model.NewError(503, "TEMPORARILY_UNAVAILABLE", "Результат записи не подтверждён; проверьте текущее состояние перед повтором")
	}
	return model.CommandResponse{Status: status, Body: body}, nil
}
