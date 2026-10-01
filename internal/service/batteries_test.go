package service

import (
	"battery-storage-pandora/internal/model"
	"testing"
)

func TestBatteryObservationErrorPrecedence(t *testing.T) {
	location := "1.2.3"
	observed := "9.9.9"
	expectedVersion := int64(2)
	battery := model.Battery{Version: 1, CurrentLocation: &location}
	input := model.BatteryCommandInput{
		ExpectedVersion:        &expectedVersion,
		ObservedSourceLocation: &observed,
	}

	err := checkBatteryObservation(battery, input)
	if err == nil || ClassifyError(err).Code != "STATE_VERSION_MISMATCH" {
		t.Fatalf("version mismatch must precede source mismatch: %v", err)
	}

	expectedVersion = battery.Version
	err = checkBatteryObservation(battery, input)
	if err == nil || ClassifyError(err).Code != "SOURCE_MISMATCH" {
		t.Fatalf("matching version must expose source mismatch: %v", err)
	}
}
