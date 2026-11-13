// Package render turns a profile into a values tree for the data-space-connector chart.
package render

import (
	"context"
	"errors"

	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

// ErrNotImplemented is returned until rendering lands with CT-2.
var ErrNotImplemented = errors.New("rendering a values.yaml is not implemented yet")

// Render produces the values.yaml for p.
func Render(_ context.Context, _ profile.Profile) ([]byte, error) {
	return nil, ErrNotImplemented
}
