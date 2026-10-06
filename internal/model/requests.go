package model

// CommandResponse is returned after the transaction commits.
type CommandResponse struct {
	Status int
	Body   []byte
}

type CreateEmployeeInput struct {
	DisplayName     string
	PersonnelNumber *string
}

type PatchEmployeeInput struct {
	DisplayName *string
	IsActive    *bool
}

type CreateCredentialInput struct {
	Value                string
	ReplacesCredentialID *int64
}

type RegisterBatteryInput struct {
	InventoryCode       string
	SerialNumber        *string
	ActorCredential     string
	DestinationLocation string
}

type BatteryCommandInput struct {
	ActorCredential     string
	DestinationLocation string
}

type CredentialResolution struct {
	Employee     Employee `json:"employee"`
	CredentialID int64    `json:"credential_id"`
}

// Filters holds validated query parameters; ID filters are already parsed to int64.
type Filters map[string]any
