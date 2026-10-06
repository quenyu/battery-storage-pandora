package service

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/repository"
	"context"
)

// All commands and card changes lock the employee before the credential.
func commandActor(ctx context.Context, tx *repository.Tx, value string) (model.Employee, model.Credential, error) {
	credential, err := tx.FindCredential(ctx, value)
	if err != nil {
		return model.Employee{}, model.Credential{}, credentialNotFound(err)
	}
	employee, err := tx.EmployeeForShare(ctx, credential.EmployeeID)
	if err != nil {
		return model.Employee{}, model.Credential{}, err
	}
	credential, err = tx.CredentialForShare(ctx, employee.ID, credential.ID)
	if err != nil {
		return model.Employee{}, model.Credential{}, err
	}
	return employee, credential, checkActive(employee, credential)
}

func (s *Service) ResolveCredential(ctx context.Context, value string) (model.CredentialResolution, error) {
	credential, err := s.repo.FindCredential(ctx, value)
	if err != nil {
		return model.CredentialResolution{}, credentialNotFound(err)
	}
	employee, err := s.repo.GetEmployeeByID(ctx, credential.EmployeeID)
	if err != nil {
		return model.CredentialResolution{}, err
	}
	if err := checkActive(employee, credential); err != nil {
		return model.CredentialResolution{}, err
	}
	return model.CredentialResolution{Employee: employee, CredentialID: credential.ID}, nil
}

func credentialNotFound(err error) error {
	if ClassifyError(err).Code == "RESOURCE_NOT_FOUND" {
		return model.NewError(404, "CREDENTIAL_NOT_FOUND", "Карта не найдена")
	}
	return err
}

func checkActive(employee model.Employee, credential model.Credential) error {
	if !employee.IsActive {
		return model.NewError(403, "EMPLOYEE_INACTIVE", "Сотрудник отключён")
	}
	if !credential.IsActive {
		return model.NewError(403, "CREDENTIAL_INACTIVE", "Карта отключена")
	}
	return nil
}
