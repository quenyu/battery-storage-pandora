package service

import (
	"battery-storage-pandora/internal/model"
	"context"
)

func (s *Service) GetEmployee(ctx context.Context, id string) (model.Employee, error) {
	return s.repo.GetEmployee(ctx, id)
}

func (s *Service) GetBattery(ctx context.Context, id string) (model.Battery, error) {
	return s.repo.GetBattery(ctx, id)
}

func (s *Service) GetOperation(ctx context.Context, id string) (model.Operation, error) {
	return s.repo.GetOperation(ctx, id)
}

func (s *Service) ListEmployees(ctx context.Context, options model.ListOptions) (model.Page, error) {
	return s.repo.ListEmployees(ctx, options)
}

func (s *Service) ListCredentials(ctx context.Context, employeeID string, options model.ListOptions) (model.Page, error) {
	if _, err := s.repo.GetEmployee(ctx, employeeID); err != nil {
		return model.Page{}, err
	}
	return s.repo.ListCredentials(ctx, employeeID, options)
}

func (s *Service) ListBatteries(ctx context.Context, options model.ListOptions) (model.Page, error) {
	return s.repo.ListBatteries(ctx, options)
}

func (s *Service) ListCustody(ctx context.Context, employeeID string, options model.ListOptions) (model.Page, error) {
	if _, err := s.repo.GetEmployee(ctx, employeeID); err != nil {
		return model.Page{}, err
	}
	return s.repo.ListCustody(ctx, employeeID, options)
}

func (s *Service) ListBatteryOperations(ctx context.Context, batteryID string, options model.ListOptions) (model.Page, error) {
	if _, err := s.repo.GetBattery(ctx, batteryID); err != nil {
		return model.Page{}, err
	}
	return s.repo.ListBatteryOperations(ctx, batteryID, options)
}

func (s *Service) ListEmployeeOperations(ctx context.Context, employeeID string, options model.ListOptions) (model.Page, error) {
	if _, err := s.repo.GetEmployee(ctx, employeeID); err != nil {
		return model.Page{}, err
	}
	return s.repo.ListEmployeeOperations(ctx, employeeID, options)
}

func (s *Service) ListOperations(ctx context.Context, options model.ListOptions) (model.Page, error) {
	return s.repo.ListOperations(ctx, options)
}
