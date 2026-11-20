// Package trialdata embeds the local trial: its descriptor and the bundled values of each release (ADR 0016).
package trialdata

import "embed"

// FS holds trial.yaml and values/<release>.yaml, byte-identical copies of the k3s/ files.
//
//go:embed trial.yaml values/*.yaml
var FS embed.FS
