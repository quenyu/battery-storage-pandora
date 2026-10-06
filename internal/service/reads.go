package service

import (
	"battery-storage-pandora/internal/model"
	"context"
)

func (s *Service) GetEmployeeByID(ctx context.Context, id int64) (model.Employee, error) {
	return s.repo.GetEmployeeByID(ctx, id)
}

func (s *Service) GetBatteryByID(ctx context.Context, id int64) (model.Battery, error) {
	return s.repo.GetBatteryByID(ctx, id)
}

func (s *Service) GetOperationByID(ctx context.Context, id int64) (model.Operation, error) {
	return s.repo.GetOperationByID(ctx, id)
}

func (s *Service) ListEmployees(ctx context.Context, filters model.Filters) ([]model.Employee, error) {
	return s.repo.ListEmployees(ctx, filters)
}

func (s *Service) ListCredentials(ctx context.Context, employeeID int64) ([]model.Credential, error) {
	if _, err := s.repo.GetEmployeeByID(ctx, employeeID); err != nil {
		return nil, err
	}
	return s.repo.ListCredentials(ctx, employeeID)
}

func (s *Service) ListBatteries(ctx context.Context, filters model.Filters) ([]model.Battery, error) {
	return s.repo.ListBatteries(ctx, filters)
}

func (s *Service) ListCustody(ctx context.Context, employeeID int64) ([]model.Battery, error) {
	if _, err := s.repo.GetEmployeeByID(ctx, employeeID); err != nil {
		return nil, err
	}
	return s.repo.ListBatteries(ctx, model.Filters{"holder_employee_id": employeeID})
}

func (s *Service) ListBatteryOperations(ctx context.Context, batteryID int64) ([]model.Operation, error) {
	if _, err := s.repo.GetBatteryByID(ctx, batteryID); err != nil {
		return nil, err
	}
	return s.repo.ListBatteryOperations(ctx, batteryID)
}

func (s *Service) ListEmployeeOperations(ctx context.Context, employeeID int64) ([]model.Operation, error) {
	if _, err := s.repo.GetEmployeeByID(ctx, employeeID); err != nil {
		return nil, err
	}
	return s.repo.ListOperations(ctx, model.Filters{"employee_id": employeeID})
}

func (s *Service) ListOperations(ctx context.Context, filters model.Filters) ([]model.Operation, error) {
	return s.repo.ListOperations(ctx, filters)
}
