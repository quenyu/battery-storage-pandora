package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type Cursor struct {
	Scope        string `json:"scope"`
	Mode         string `json:"mode"`
	ID           string `json:"id"`
	Time         string `json:"time,omitempty"`
	Version      int64  `json:"version,omitempty"`
	UpperID      string `json:"upper_id,omitempty"`
	UpperTime    string `json:"upper_time,omitempty"`
	UpperVersion int64  `json:"upper_version,omitempty"`
}

func EncodeCursor(c Cursor) string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(sum[:])
}

func DecodeCursor(token string) (Cursor, error) {
	var c Cursor
	if len(token) > 4096 {
		return c, fmt.Errorf("cursor too large")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return c, fmt.Errorf("cursor envelope")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, err
	}
	sum := sha256.Sum256(raw)
	if parts[1] != hex.EncodeToString(sum[:]) {
		return c, fmt.Errorf("cursor checksum")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("cursor trailing data")
	}
	if !ValidUUID(c.ID) {
		return c, fmt.Errorf("cursor id")
	}
	switch c.Mode {
	case "id":
		if c.Time != "" || c.Version != 0 || c.UpperID != "" || c.UpperTime != "" || c.UpperVersion != 0 {
			return c, fmt.Errorf("cursor fields")
		}
	case "version":
		if c.Version < 1 || c.UpperVersion < c.Version || c.Time != "" || c.UpperTime != "" || c.UpperID != "" {
			return c, fmt.Errorf("cursor version")
		}
	case "time":
		t, err := time.Parse(time.RFC3339Nano, c.Time)
		if err != nil {
			return c, err
		}
		u, err := time.Parse(time.RFC3339Nano, c.UpperTime)
		if err != nil {
			return c, err
		}
		if !ValidUUID(c.UpperID) || u.Before(t) || (u.Equal(t) && c.UpperID < c.ID) || c.Version != 0 || c.UpperVersion != 0 {
			return c, fmt.Errorf("cursor boundary")
		}
	default:
		return c, fmt.Errorf("cursor mode")
	}
	return c, nil
}
