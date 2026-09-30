package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

// Commands and card changes lock employee first, then credential.
func commandActor(ctx context.Context, tx *repository.Tx, value string) (model.Employee, model.Credential, error) {
	credential, err := tx.FindCredential(ctx, value)
	if err != nil {
		if ClassifyError(err).Code == "RESOURCE_NOT_FOUND" {
			err = model.NewError(404, "CREDENTIAL_NOT_FOUND", "Карта не найдена")
		}
		return model.Employee{}, model.Credential{}, err
	}
	employee, err := tx.EmployeeForShare(ctx, credential.EmployeeID)
	if err != nil {
		return model.Employee{}, model.Credential{}, err
	}
	if !employee.IsActive {
		return model.Employee{}, model.Credential{}, model.NewError(403, "EMPLOYEE_INACTIVE", "Сотрудник отключён")
	}
	credential, err = tx.CredentialForShare(ctx, employee.ID, credential.ID)
	if err != nil {
		return model.Employee{}, model.Credential{}, err
	}
	if !credential.IsActive {
		return model.Employee{}, model.Credential{}, model.NewError(403, "CREDENTIAL_INACTIVE", "Карта отключена")
	}
	return employee, credential, nil
}

func (s *Service) ResolveCredential(ctx context.Context, value string) (model.CredentialResolution, error) {
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return model.CredentialResolution{}, err
	}
	defer tx.Rollback()
	employee, credential, err := commandActor(ctx, tx, value)
	if err != nil {
		return model.CredentialResolution{}, err
	}
	return model.CredentialResolution{Employee: employee, CredentialID: credential.ID}, nil
}
