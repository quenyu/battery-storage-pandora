package model

import "time"

type Employee struct {
	ID              string     `json:"id"`
	PersonnelNumber *string    `json:"personnel_number"`
	DisplayName     string     `json:"display_name"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DisabledAt      *time.Time `json:"disabled_at"`
}

type Credential struct {
	ID         string     `json:"id"`
	EmployeeID string     `json:"employee_id"`
	Value      string     `json:"value"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DisabledAt *time.Time `json:"disabled_at"`
}

type Battery struct {
	ID                      string    `json:"id"`
	InventoryCode           string    `json:"inventory_code"`
	SerialNumber            *string   `json:"serial_number"`
	Status                  string    `json:"status"`
	CurrentLocation         *string   `json:"current_location"`
	CurrentHolderEmployeeID *string   `json:"current_holder_employee_id"`
	Version                 int64     `json:"version"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type Operation struct {
	ID                          string    `json:"id"`
	BatteryID                   string    `json:"battery_id"`
	BatteryVersion              int64     `json:"battery_version"`
	Type                        string    `json:"type"`
	ActorEmployeeID             string    `json:"actor_employee_id"`
	CredentialID                string    `json:"credential_id"`
	SourceStatus                *string   `json:"source_status"`
	DestinationStatus           string    `json:"destination_status"`
	SourceLocation              *string   `json:"source_location"`
	DestinationLocation         *string   `json:"destination_location"`
	SourceHolderEmployeeID      *string   `json:"source_holder_employee_id"`
	DestinationHolderEmployeeID *string   `json:"destination_holder_employee_id"`
	DeviceCode                  *string   `json:"device_code"`
	OccurredAt                  time.Time `json:"occurred_at"`
}

type CommandResult struct {
	Battery   Battery   `json:"battery"`
	Operation Operation `json:"operation"`
}

type Page struct {
	Items      any     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
