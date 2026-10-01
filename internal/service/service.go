package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"

	"github.com/google/uuid"
)

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func newID() string {
	return uuid.NewString()
}

func ClassifyError(err error) *model.Error {
	return repository.ClassifyError(err)
}
