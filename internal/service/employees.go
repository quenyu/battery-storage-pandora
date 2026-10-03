package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

func (s *Service) CreateEmployee(ctx context.Context, input model.CreateEmployeeInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, 201, func(tx *repository.Tx) (any, error) {
		employeeID := newID()
		if err := tx.InsertEmployee(ctx, employeeID, input); err != nil {
			return nil, err
		}
		return tx.GetEmployee(ctx, employeeID)
	})
}

func (s *Service) PatchEmployee(ctx context.Context, employeeID string, input model.PatchEmployeeInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, 200, func(tx *repository.Tx) (any, error) {
		if input.DisplayName == nil && input.IsActive == nil {
			return nil, model.NewError(422, "VALIDATION_FAILED", "Минимум одно поле должно быть заполнено")
		}
		employee, err := tx.LockEmployee(ctx, employeeID)
		if err != nil {
			return nil, err
		}
		if input.DisplayName != nil {
			employee.DisplayName = *input.DisplayName
		}
		if input.IsActive != nil && *input.IsActive {
			employee.DisabledAt = nil
		}
		if input.IsActive != nil && !*input.IsActive {
			hasBatteries, err := tx.HasCustody(ctx, employeeID)
			if err != nil {
				return nil, err
			}
			if hasBatteries {
				return nil, model.Conflict("EMPLOYEE_HAS_CUSTODY")
			}
			if employee.DisabledAt == nil {
				disabledAt, err := tx.Now(ctx)
				if err != nil {
					return nil, err
				}
				employee.DisabledAt = &disabledAt
			}
		}
		if err := tx.UpdateEmployee(ctx, employee); err != nil {
			return nil, err
		}
		return tx.GetEmployee(ctx, employeeID)
	})
}
