package model

import "time"

const (
	BatteryStored = "STORED"
	BatteryIssued = "ISSUED"
)

const (
	OperationStore  = "STORE"
	OperationTake   = "TAKE"
	OperationReturn = "RETURN"
	OperationMove   = "MOVE"
)

type Employee struct {
	ID              int64      `json:"id"`
	PersonnelNumber *string    `json:"personnel_number"`
	DisplayName     string     `json:"display_name"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DisabledAt      *time.Time `json:"disabled_at"`
}

type Credential struct {
	ID         int64      `json:"id"`
	EmployeeID int64      `json:"employee_id"`
	Value      string     `json:"value"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	DisabledAt *time.Time `json:"disabled_at"`
}

// Battery state (status, location, holder) is derived from its latest operation.
type Battery struct {
	ID                      int64     `json:"id"`
	InventoryCode           string    `json:"inventory_code"`
	SerialNumber            *string   `json:"serial_number"`
	Status                  string    `json:"status"`
	CurrentLocation         *string   `json:"current_location"`
	CurrentHolderEmployeeID *int64    `json:"current_holder_employee_id"`
	CreatedAt               time.Time `json:"created_at"`
}

// Operation is one history entry. The actor is the owner of the credential and
// the source location is the location of the previous operation of the battery.
type Operation struct {
	ID                  int64     `json:"id"`
	BatteryID           int64     `json:"battery_id"`
	Type                string    `json:"type"`
	ActorEmployeeID     int64     `json:"actor_employee_id"`
	CredentialID        int64     `json:"credential_id"`
	SourceLocation      *string   `json:"source_location"`
	DestinationLocation *string   `json:"destination_location"`
	OccurredAt          time.Time `json:"occurred_at"`
}

type CommandResult struct {
	Battery   Battery   `json:"battery"`
	Operation Operation `json:"operation"`
}

type ListResult struct {
	Items any `json:"items"`
}
