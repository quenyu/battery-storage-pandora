package api

import "time"

type Credential struct {
	ID         string     `json:"id"`
	EmployeeID string     `json:"employee_id"`
	Barcode    string     `json:"barcode"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}
type Employee struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	CreatedAt        time.Time  `json:"created_at"`
	ActiveCredential Credential `json:"active_credential"`
}
type Cabinet struct {
	ID     string `json:"id"`
	Number int32  `json:"number"`
}
type Shelf struct {
	ID        string `json:"id"`
	CabinetID string `json:"cabinet_id"`
	Number    int32  `json:"number"`
}
type Cell struct {
	ID        string  `json:"id"`
	ShelfID   string  `json:"shelf_id"`
	Number    int32   `json:"number"`
	Address   string  `json:"address"`
	BatteryID *string `json:"battery_id"`
}
type Battery struct {
	ID               string    `json:"id"`
	InventoryCode    string    `json:"inventory_code"`
	Status           string    `json:"status"`
	CellID           *string   `json:"cell_id"`
	HolderEmployeeID *string   `json:"holder_employee_id"`
	Version          int32     `json:"version"`
	LastOperationID  string    `json:"last_operation_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Operation struct {
	ID                   string    `json:"id"`
	BatteryID            string    `json:"battery_id"`
	BatteryVersion       int32     `json:"battery_version"`
	Type                 string    `json:"type"`
	ActorEmployeeID      string    `json:"actor_employee_id"`
	CredentialID         string    `json:"credential_id"`
	FromCellID           *string   `json:"from_cell_id"`
	ToCellID             *string   `json:"to_cell_id"`
	FromHolderEmployeeID *string   `json:"from_holder_employee_id"`
	ToHolderEmployeeID   *string   `json:"to_holder_employee_id"`
	Reason               *string   `json:"reason"`
	OccurredAt           time.Time `json:"occurred_at"`
}
type CommandResult struct {
	Battery   Battery   `json:"battery"`
	Operation Operation `json:"operation"`
}
type Page struct {
	Items  any `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}
