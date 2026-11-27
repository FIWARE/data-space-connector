// Package requirements lists the cluster objects a profile's values reference but the tool never creates, so that
// NEXT_STEPS.md can name them (ADR 0025). It resolves names and keys only and never reads or holds their content.
package requirements

import (
	"fmt"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// Secret is a Secret the user creates in the release namespace before installing.
type Secret struct {
	// Namespace is the profile's target namespace; empty while Stage 1 leaves it unanswered.
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Purpose   string `json:"purpose"`
}

// Secrets returns the Secrets p's given and derived answers name, in bundle order. A requirement whose name reads an
// unanswered question is left out; the error is render.Answers'.
func Secrets(b model.Bundle, p profile.Profile) ([]Secret, error) {
	answers, err := render.Answers(b, p)
	if err != nil {
		return nil, err
	}
	var secrets []Secret
	for _, r := range b.Requirements() {
		name, ok, err := r.Resolve(answers)
		if err != nil {
			return nil, fmt.Errorf("requirement %s: %w", r.Key, err)
		}
		if ok {
			secrets = append(secrets, Secret{Namespace: p.Target.Namespace, Name: name, Key: r.Key, Purpose: r.Purpose})
		}
	}
	return secrets, nil
}
