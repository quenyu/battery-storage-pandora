package service

import (
	"battery-storage-pandora/internal/model"
	"testing"
)

func text(value string) *string { return &value }

func TestIsReplayComparesTheWholeCommand(t *testing.T) {
	last := model.Operation{Type: model.OperationMove, CredentialID: 7, DestinationLocation: text("1.1.2")}
	cases := []struct {
		name        string
		kind        string
		credential  int64
		destination *string
		want        bool
	}{
		{"same command", model.OperationMove, 7, text("1.1.2"), true},
		{"other credential of any employee", model.OperationMove, 8, text("1.1.2"), false},
		{"other destination", model.OperationMove, 7, text("1.1.3"), false},
		{"other type", model.OperationReturn, 7, text("1.1.2"), false},
	}
	for _, c := range cases {
		if got := isReplay(last, c.kind, c.credential, c.destination); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	take := model.Operation{Type: model.OperationTake, CredentialID: 7}
	if !isReplay(take, model.OperationTake, 7, nil) {
		t.Error("repeated TAKE must be a replay")
	}
}

func TestCheckTransition(t *testing.T) {
	holder := int64(1)
	stored := model.Battery{Status: model.BatteryStored, CurrentLocation: text("1.1.1")}
	issued := model.Battery{Status: model.BatteryIssued, CurrentHolderEmployeeID: &holder}
	cases := []struct {
		name        string
		battery     model.Battery
		kind        string
		actor       int64
		destination *string
		code        string
	}{
		{"take stored", stored, model.OperationTake, 2, nil, ""},
		{"take issued", issued, model.OperationTake, 2, nil, "BATTERY_ALREADY_ISSUED"},
		{"return by holder", issued, model.OperationReturn, 1, text("1.1.2"), ""},
		{"return by another employee", issued, model.OperationReturn, 2, text("1.1.2"), "RETURN_NOT_ALLOWED"},
		{"return stored", stored, model.OperationReturn, 1, text("1.1.2"), "INVALID_BATTERY_STATE"},
		{"move stored", stored, model.OperationMove, 2, text("1.1.2"), ""},
		{"move to same location", stored, model.OperationMove, 2, text("1.1.1"), "SAME_LOCATION"},
		{"move issued", issued, model.OperationMove, 1, text("1.1.2"), "INVALID_BATTERY_STATE"},
	}
	for _, c := range cases {
		err := checkTransition(c.battery, c.kind, c.actor, c.destination)
		code := ""
		if err != nil {
			code = ClassifyError(err).Code
		}
		if code != c.code {
			t.Errorf("%s: got %q, want %q", c.name, code, c.code)
		}
	}
}
