// Package bundles embeds the question-model bundle of every supported chart version. It is not
// named model so that importers of both it and internal/model need no import alias.
package bundles

import "embed"

// FS holds <chart-version>/bundle.yaml for each supported chart version.
//
//go:embed */bundle.yaml
var FS embed.FS
