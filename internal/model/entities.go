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

type User struct {
	ID         int64      `json:"id"`
	Barcode    string     `json:"barcode"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DisabledAt *time.Time `json:"disabled_at"`
}

// Battery state (status, location, holder) is derived from its latest operation.
type Battery struct {
	ID                  int64     `json:"id"`
	InventoryCode       string    `json:"inventory_code"`
	Status              string    `json:"status"`
	CurrentLocation     *string   `json:"current_location"`
	CurrentHolderUserID *int64    `json:"current_holder_user_id"`
	CreatedAt           time.Time `json:"created_at"`
}

type Operation struct {
	ID                  int64     `json:"id"`
	BatteryID           int64     `json:"battery_id"`
	Type                string    `json:"type"`
	ActorUserID         int64     `json:"actor_user_id"`
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
