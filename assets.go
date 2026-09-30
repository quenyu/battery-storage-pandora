// Package assets embeds the authoritative API contract in the server binary.
package assets

import _ "embed"

// OpenAPI is the sole editable contract, embedded directly without a generated copy.
//
//go:embed docs/design/battery-storage-discovery/openapi.yaml
var OpenAPI []byte
