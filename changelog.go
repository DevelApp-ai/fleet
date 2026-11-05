// Package aifleet holds files built into the fleet binary from the repo root.
package aifleet

import _ "embed"

// Changelog is CHANGELOG.md: fleet's user-facing changes, newest first
// (DESIGN.md §5d).
//
//go:embed CHANGELOG.md
var Changelog string
