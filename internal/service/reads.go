package service

import (
	"battery-storage-pandora/internal/model"
	"context"
)

func (s *Service) GetUserByID(ctx context.Context, id int64) (model.User, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *Service) GetBatteryByID(ctx context.Context, id int64) (model.Battery, error) {
	return s.repo.GetBatteryByID(ctx, id)
}

func (s *Service) GetOperationByID(ctx context.Context, id int64) (model.Operation, error) {
	return s.repo.GetOperationByID(ctx, id)
}

func (s *Service) ListUsers(ctx context.Context, filters model.Filters) ([]model.User, error) {
	return s.repo.ListUsers(ctx, filters)
}

func (s *Service) ListBatteries(ctx context.Context, filters model.Filters) ([]model.Battery, error) {
	return s.repo.ListBatteries(ctx, filters)
}

func (s *Service) ListOperations(ctx context.Context, filters model.Filters) ([]model.Operation, error) {
	return s.repo.ListOperations(ctx, filters)
}
