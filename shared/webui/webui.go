// Package webui provides the embedded web UI served by all camera API services.
package webui

import _ "embed"

// IndexHTML is the single-page camera dashboard, embedded at compile time.
//
//go:embed index.html
var IndexHTML []byte
