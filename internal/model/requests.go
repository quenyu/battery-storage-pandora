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
	ReplacesCredentialID *string
}

type RegisterBatteryInput struct {
	InventoryCode       string
	SerialNumber        *string
	ActorCredential     string
	DestinationLocation string
}

type BatteryCommandInput struct {
	ActorCredential        string
	DestinationLocation    string
	ExpectedVersion        *int64
	ObservedSourceLocation *string
}

type CredentialResolution struct {
	Employee     Employee `json:"employee"`
	CredentialID string   `json:"credential_id"`
}

type Filters map[string]string
