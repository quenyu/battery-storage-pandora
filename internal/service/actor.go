package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

// All commands lock the acting user first. Only an active user is found.
func commandActor(ctx context.Context, tx *repository.Tx, barcode string) (model.User, error) {
	user, err := tx.ActiveUserForShare(ctx, barcode)
	if err != nil && ClassifyError(err).Code == "RESOURCE_NOT_FOUND" {
		return user, model.NewError(404, "BARCODE_NOT_FOUND", "Активный пользователь с таким ШК не найден")
	}
	return user, err
}
