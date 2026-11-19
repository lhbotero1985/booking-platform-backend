// Package api embeds the OpenAPI 3.1 contract (openapi.yaml), the source of truth for the HTTP
// API (ADR-0011). Contract tests validate real responses against it.
package api

import _ "embed"

// Spec is the raw OpenAPI document.
//
//go:embed openapi.yaml
var Spec []byte
