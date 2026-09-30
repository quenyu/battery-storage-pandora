package service

import (
	"battery-storage-pandora/internal/model"
	"context"
)

func (s *Service) GetEmployeeByID(ctx context.Context, id string) (model.Employee, error) {
	return s.repo.GetEmployeeByID(ctx, id)
}

func (s *Service) GetBatteryByID(ctx context.Context, id string) (model.Battery, error) {
	return s.repo.GetBatteryByID(ctx, id)
}

func (s *Service) GetOperationByID(ctx context.Context, id string) (model.Operation, error) {
	return s.repo.GetOperationByID(ctx, id)
}

func (s *Service) ListEmployees(ctx context.Context, filters model.Filters) ([]model.Employee, error) {
	return s.repo.ListEmployees(ctx, filters)
}

func (s *Service) ListCredentials(ctx context.Context, resourceID string) ([]model.Credential, error) {
	if _, err := s.repo.GetEmployeeByID(ctx, resourceID); err != nil {
		return nil, err
	}
	return s.repo.ListCredentials(ctx, resourceID)
}

func (s *Service) ListBatteries(ctx context.Context, filters model.Filters) ([]model.Battery, error) {
	return s.repo.ListBatteries(ctx, filters)
}

func (s *Service) ListCustody(ctx context.Context, resourceID string) ([]model.Battery, error) {
	if _, err := s.repo.GetEmployeeByID(ctx, resourceID); err != nil {
		return nil, err
	}
	return s.repo.ListCustody(ctx, resourceID)
}

func (s *Service) ListBatteryOperations(ctx context.Context, resourceID string) ([]model.Operation, error) {
	if _, err := s.repo.GetBatteryByID(ctx, resourceID); err != nil {
		return nil, err
	}
	return s.repo.ListBatteryOperations(ctx, resourceID)
}

func (s *Service) ListEmployeeOperations(ctx context.Context, resourceID string) ([]model.Operation, error) {
	if _, err := s.repo.GetEmployeeByID(ctx, resourceID); err != nil {
		return nil, err
	}
	return s.repo.ListEmployeeOperations(ctx, resourceID)
}

func (s *Service) ListOperations(ctx context.Context, filters model.Filters) ([]model.Operation, error) {
	return s.repo.ListOperations(ctx, filters)
}
