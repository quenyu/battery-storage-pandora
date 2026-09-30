package model

// CommandRequest identifies an idempotent command after HTTP validation.
type CommandRequest struct {
	Key       string
	Hash      string
	RequestID string
}

// CommandResponse is stored together with the operation and battery state.
type CommandResponse struct {
	Status   int
	Body     []byte
	Replayed bool
}

type SavedRequest struct {
	Hash   string
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
