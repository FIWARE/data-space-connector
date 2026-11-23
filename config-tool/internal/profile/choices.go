package profile

import (
	"fmt"
	"maps"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

// FeatureChoice is the state of one feature next to its preset value; the UI words it (ADR 0018).
type FeatureChoice struct {
	ID      string      `json:"id"`
	Offer   model.Offer `json:"offer"`
	Preset  bool        `json:"preset"`
	Enabled bool        `json:"enabled"`
	// Deliberate is set when Enabled differs from Preset and the state did not come from the preset.
	Deliberate bool          `json:"deliberate"`
	Source     FeatureSource `json:"source"`
}

// SetFeature records the user's toggle of feature id as source user, whatever decided it before.
// It never modifies p's maps and returns p unchanged with any error.
func (p Profile) SetFeature(bundles []model.Bundle, id string, enabled bool) (Profile, error) {
	cat, r, err := p.roleCatalogue(bundles)
	if err != nil {
		return p, err
	}
	if _, ok := r.Features[id]; !ok {
		return p, fmt.Errorf("%w %q", ErrUnknownFeature, id)
	}
	if !inScope(cat, r, id) {
		return p, fmt.Errorf("%w: %s sets values of chart %s, role %s deploys chart %s", ErrFeatureOfOtherChart, id, featureChart(cat, id), r.ID, r.Chart)
	}
	out := p
	out.Features = maps.Clone(p.Features)
	if out.Features == nil {
		out.Features = map[string]FeatureState{}
	}
	out.Features[id] = FeatureState{Enabled: enabled, Source: SourceUser}
	return out, nil
}

// FeatureChoices lists every feature of the role's chart in catalogue order; an unrecorded feature
// reports its preset value from the preset.
func (p Profile) FeatureChoices(bundles []model.Bundle) ([]FeatureChoice, error) {
	cat, r, err := p.roleCatalogue(bundles)
	if err != nil {
		return nil, err
	}
	choices := []FeatureChoice{}
	for _, f := range cat.Features {
		o, ok := r.Features[f.ID]
		if !ok || !inScope(cat, r, f.ID) {
			continue
		}
		preset := o == model.OfferRecommended
		s := effectiveState(r, f.ID, p.Features)
		choices = append(choices, FeatureChoice{
			ID: f.ID, Offer: o, Preset: preset, Enabled: s.Enabled, Source: s.Source,
			Deliberate: s.Enabled != preset && s.Source != SourcePreset,
		})
	}
	return choices, nil
}

// effectiveState is feature id's recorded state, else role r's preset value (off when r does not list id).
// FeatureChoices and RoleChangeTo share it, so Stage 5 and the role-change summary agree (ADR 0019).
func effectiveState(r *model.Role, id string, recorded map[string]FeatureState) FeatureState {
	if s, ok := recorded[id]; ok {
		return s
	}
	return FeatureState{Enabled: r.Features[id] == model.OfferRecommended, Source: SourcePreset}
}

// roleCatalogue resolves the profile's role catalogue and role.
func (p Profile) roleCatalogue(bundles []model.Bundle) (model.Bundle, *model.Role, error) {
	if p.Role == "" {
		return model.Bundle{}, nil, ErrNoRole
	}
	cat, err := CatalogueBundle(bundles, p.Model, p.Catalogue)
	if err != nil {
		return model.Bundle{}, nil, err
	}
	r := findRole(cat, p.Role)
	if r == nil {
		return model.Bundle{}, nil, fmt.Errorf("%w %q in bundle %s@%s", ErrUnknownRole, p.Role, cat.Chart, cat.Version)
	}
	return cat, r, nil
}
