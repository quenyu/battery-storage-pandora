package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

func (s *Service) CreateEmployee(ctx context.Context, input model.CreateEmployeeInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		employeeID, err := tx.InsertEmployee(ctx, input)
		if err != nil {
			return 0, nil, err
		}
		employee, err := tx.GetEmployee(ctx, employeeID)
		return 201, employee, err
	})
}

func (s *Service) PatchEmployee(ctx context.Context, employeeID int64, input model.PatchEmployeeInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		if input.DisplayName == nil && input.IsActive == nil {
			return 0, nil, model.NewError(422, "VALIDATION_FAILED", "Минимум одно поле должно быть заполнено")
		}
		// The exclusive lock waits for battery commands that hold this employee in share mode.
		employee, err := tx.LockEmployee(ctx, employeeID)
		if err != nil {
			return 0, nil, err
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
				return 0, nil, err
			}
			if hasBatteries {
				return 0, nil, model.Conflict("EMPLOYEE_HAS_CUSTODY")
			}
			if employee.DisabledAt == nil {
				disabledAt, err := tx.Now(ctx)
				if err != nil {
					return 0, nil, err
				}
				employee.DisabledAt = &disabledAt
			}
		}
		if err := tx.UpdateEmployee(ctx, employee); err != nil {
			return 0, nil, err
		}
		employee, err = tx.GetEmployee(ctx, employeeID)
		return 200, employee, err
	})
}
