// Package validate checks a values.yaml against the chart's values schema.
package validate

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned until validation lands with a later ticket.
var ErrNotImplemented = errors.New("validating a values.yaml is not implemented yet")

// Validate checks the given values.yaml document.
func Validate(_ context.Context, _ []byte) error {
	return ErrNotImplemented
}
