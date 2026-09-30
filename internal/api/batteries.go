package api

import (
	"context"
	"database/sql"
	"net/http"
)

func registerBattery(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
	eid, cid, err := commandActor(ctx, tx, in.str("actor_credential_value"))
	if err != nil {
		return nil, err
	}
	id := newID()
	var serial any
	if v, ok := in["serial_number"]; ok {
		serial = v
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO batteries(id,inventory_code,serial_number,status,current_location,version) VALUES($1,$2,$3,'STORED',$4,1)`, id, in.str("inventory_code"), serial, in.str("destination_location")); err != nil {
		return nil, err
	}
	dest := in.str("destination_location")
	o := Operation{ID: newID(), BatteryID: id, BatteryVersion: 1, Type: "STORE", ActorEmployeeID: eid, CredentialID: cid, DestinationStatus: "STORED", DestinationLocation: &dest}
	return finishBatteryCommand(ctx, tx, r, o)
}
func batteryCommand(kind string) command {
	return func(ctx context.Context, tx *sql.Tx, r *http.Request, in input) (any, error) {
		eid, cid, err := commandActor(ctx, tx, in.str("actor_credential_value"))
		if err != nil {
			return nil, err
		}
		b, err := scanBattery(tx.QueryRowContext(ctx, batterySelect+` WHERE b.id=$1 FOR UPDATE`, r.PathValue("battery_id")))
		if err != nil {
			return nil, err
		}
		if expected, ok := in["expected_version"]; ok && expected.(int64) != b.Version {
			return nil, conflict("STATE_VERSION_MISMATCH")
		}
		if observed, ok := in["observed_source_location"]; ok && (b.CurrentLocation == nil || *b.CurrentLocation != observed.(string)) {
			return nil, conflict("SOURCE_MISMATCH")
		}
		o := Operation{ID: newID(), BatteryID: b.ID, BatteryVersion: b.Version + 1, Type: kind, ActorEmployeeID: eid, CredentialID: cid, SourceStatus: &b.Status, SourceLocation: b.CurrentLocation, SourceHolderEmployeeID: b.CurrentHolderEmployeeID}
		switch kind {
		case "TAKE":
			if b.Status == "ISSUED" {
				return nil, conflict("BATTERY_ALREADY_ISSUED")
			}
			if b.Status != "STORED" {
				return nil, conflict("INVALID_BATTERY_STATE")
			}
			o.DestinationStatus = "ISSUED"
			o.DestinationHolderEmployeeID = &eid
		case "RETURN":
			if b.Status != "ISSUED" {
				return nil, conflict("INVALID_BATTERY_STATE")
			}
			if b.CurrentHolderEmployeeID == nil || *b.CurrentHolderEmployeeID != eid {
				return nil, fail(403, "RETURN_NOT_ALLOWED", "Вернуть АКБ может только взявший сотрудник")
			}
			dest := in.str("destination_location")
			o.DestinationStatus = "STORED"
			o.DestinationLocation = &dest
		case "MOVE":
			if b.Status != "STORED" {
				return nil, conflict("INVALID_BATTERY_STATE")
			}
			dest := in.str("destination_location")
			if b.CurrentLocation != nil && *b.CurrentLocation == dest {
				return nil, conflict("SAME_LOCATION")
			}
			o.DestinationStatus = "STORED"
			o.DestinationLocation = &dest
		default:
			return nil, fail(500, "INTERNAL_ERROR", "Неизвестная команда")
		}
		// The unique index is authoritative for destination occupancy across batteries.
		if _, err := tx.ExecContext(ctx, `UPDATE batteries SET status=$2,current_location=$3,current_holder_employee_id=$4,version=$5 WHERE id=$1`, b.ID, o.DestinationStatus, o.DestinationLocation, o.DestinationHolderEmployeeID, o.BatteryVersion); err != nil {
			return nil, err
		}
		return finishBatteryCommand(ctx, tx, r, o)
	}
}
func finishBatteryCommand(ctx context.Context, tx *sql.Tx, r *http.Request, o Operation) (CommandResult, error) {
	p := r.Context().Value(principalKey{}).(Principal)
	o.DeviceCode = p.DeviceCode
	var result CommandResult
	err := tx.QueryRowContext(ctx, `INSERT INTO battery_operations(id,battery_id,battery_version,type,actor_employee_id,credential_id,source_status,destination_status,source_location,destination_location,source_holder_employee_id,destination_holder_employee_id,device_code,request_scope,request_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING occurred_at`, o.ID, o.BatteryID, o.BatteryVersion, o.Type, o.ActorEmployeeID, o.CredentialID, o.SourceStatus, o.DestinationStatus, o.SourceLocation, o.DestinationLocation, o.SourceHolderEmployeeID, o.DestinationHolderEmployeeID, o.DeviceCode, p.Scope, r.Header.Get("Idempotency-Key")).Scan(&o.OccurredAt)
	if err != nil {
		return result, err
	}
	o.OccurredAt = o.OccurredAt.UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE batteries SET updated_at=$2 WHERE id=$1`, o.BatteryID, o.OccurredAt); err != nil {
		return result, err
	}
	result.Operation = o
	result.Battery, err = getBattery(ctx, tx, o.BatteryID)
	return result, err
}
