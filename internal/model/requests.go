package model

// CommandResponse is returned after the transaction commits.
type CommandResponse struct {
	Status int
	Body   []byte
}

type CreateUserInput struct {
	Name    string
	Barcode string
}

// PatchUserInput can rename or disable a user; IsActive is only ever false.
type PatchUserInput struct {
	Name     *string
	IsActive *bool
}

type RegisterBatteryInput struct {
	InventoryCode       string
	ActorBarcode        string
	DestinationLocation string
}

type BatteryCommandInput struct {
	ActorBarcode        string
	DestinationLocation string
}

// Filters holds validated query parameters; ID filters are already parsed to int64.
type Filters map[string]any
