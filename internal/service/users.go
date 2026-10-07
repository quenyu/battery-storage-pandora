package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

func (s *Service) CreateUser(ctx context.Context, input model.CreateUserInput) (model.CommandResponse, error) {
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		userID, err := tx.InsertUser(ctx, input)
		if err != nil {
			return 0, nil, err
		}
		user, err := tx.GetUser(ctx, userID)
		return 201, user, err
	})
}

// PatchUser renames or disables a user. Disabling is permanent.
func (s *Service) PatchUser(ctx context.Context, userID int64, input model.PatchUserInput) (model.CommandResponse, error) {
	if input.Name == nil && input.IsActive == nil {
		return model.CommandResponse{}, model.NewError(422, "VALIDATION_FAILED", "Минимум одно поле должно быть заполнено")
	}
	if input.IsActive != nil && *input.IsActive {
		return model.CommandResponse{}, model.NewError(422, "VALIDATION_FAILED", "Пользователя можно только отключить; включение не поддерживается")
	}
	return s.runCommand(ctx, func(tx *repository.Tx) (int, any, error) {
		// The exclusive lock waits for battery commands that hold this user in share mode.
		user, err := tx.LockUser(ctx, userID)
		if err != nil {
			return 0, nil, err
		}
		if input.Name != nil {
			user.Name = *input.Name
		}
		if input.IsActive != nil {
			hasBatteries, err := tx.HasCustody(ctx, userID)
			if err != nil {
				return 0, nil, err
			}
			if hasBatteries {
				return 0, nil, model.Conflict("USER_HAS_CUSTODY")
			}
			if user.DisabledAt == nil {
				disabledAt, err := tx.Now(ctx)
				if err != nil {
					return 0, nil, err
				}
				user.DisabledAt = &disabledAt
			}
		}
		if err := tx.UpdateUser(ctx, user); err != nil {
			return 0, nil, err
		}
		user, err = tx.GetUser(ctx, userID)
		return 200, user, err
	})
}
