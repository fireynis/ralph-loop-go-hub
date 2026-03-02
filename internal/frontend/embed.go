package frontend

import "embed"

// Dist contains the static frontend build output.
// Populated by copying web/out/ to internal/frontend/dist/ before building.
//
//go:embed all:dist
var Dist embed.FS
