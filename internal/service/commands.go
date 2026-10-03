package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
	"encoding/json"
)

type command func(*repository.Tx) (any, error)

func (s *Service) runCommand(ctx context.Context, status int, execute command) (model.CommandResponse, error) {
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return model.CommandResponse{}, err
	}
	defer tx.Rollback()
	value, err := execute(tx)
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
