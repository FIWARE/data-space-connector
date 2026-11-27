// Package requirements lists the cluster objects a profile's values reference but the tool never creates, so that
// NEXT_STEPS.md can name them (ADR 0025). It resolves names and keys only and never reads or holds their content.
package requirements

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// Secret is a Secret the user creates in the release namespace before installing.
type Secret struct {
	// Namespace is the profile's target namespace; empty while Stage 1 leaves it unanswered.
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	// Keys are the data keys the Secret must hold.
	Keys    []string `json:"keys"`
	Purpose string   `json:"purpose"`
}

// Secrets returns the Secrets p's given and derived answers name, each once, in bundle order. A requirement whose name
// reads an unanswered question, or whose When does not hold, is left out; rows naming the same Secret merge their keys
// and keep the first purpose. The error is render.Answers'.
func Secrets(b model.Bundle, p profile.Profile) ([]Secret, error) {
	answers, err := render.Answers(b, p)
	if err != nil {
		return nil, err
	}
	var secrets []Secret
	for _, r := range b.Requirements() {
		name, ok, err := r.Resolve(answers)
		if err != nil {
			return nil, fmt.Errorf("requirement %s: %w", strings.Join(r.Keys, ","), err)
		}
		if !ok {
			continue
		}
		i := slices.IndexFunc(secrets, func(s Secret) bool { return s.Name == name })
		if i < 0 {
			secrets = append(secrets, Secret{Namespace: p.Target.Namespace, Name: name, Purpose: r.Purpose})
			i = len(secrets) - 1
		}
		for _, k := range r.Keys {
			if !slices.Contains(secrets[i].Keys, k) {
				secrets[i].Keys = append(secrets[i].Keys, k)
			}
		}
	}
	return secrets, nil
}
