package profile

import (
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

// RoleChange is the summary shown before a role change is applied (US-2.3, ADR 0019); it holds ids only.
// Its slices are never nil and follow the new role's catalogue order, unknown ids sorted after it.
type RoleChange struct {
	FromRole string `json:"fromRole"`
	ToRole   string `json:"toRole"`
	// Enabled and Disabled are the features whose effective state went off -> on and on -> off.
	Enabled  []string `json:"enabled"`
	Disabled []string `json:"disabled"`
	// Kept are the user and loaded toggles of the new profile that keep their state.
	Kept []KeptFeature `json:"kept"`
}

// KeptFeature is a toggle a role change keeps; Source tells the UI who set it.
type KeptFeature struct {
	ID      string        `json:"id"`
	Enabled bool          `json:"enabled"`
	Source  FeatureSource `json:"source"`
}

// RoleChangeTo summarises the change from p to after, the ApplyPreset result; ErrNoRole when p has no role.
// The same effective-state rule covers a change within a chart and a chart switch.
func (p Profile) RoleChangeTo(bundles []model.Bundle, after Profile) (RoleChange, error) {
	_, from, err := p.roleCatalogue(bundles)
	if err != nil {
		return RoleChange{}, err
	}
	cat, to, err := after.roleCatalogue(bundles)
	if err != nil {
		return RoleChange{}, err
	}
	ids := map[string]bool{}
	for _, r := range []*model.Role{from, to} {
		for id := range r.Features {
			ids[id] = true
		}
	}
	for _, m := range []map[string]FeatureState{p.Features, after.Features} {
		for id := range m {
			ids[id] = true
		}
	}

	rc := RoleChange{FromRole: p.Role, ToRole: after.Role, Enabled: []string{}, Disabled: []string{}, Kept: []KeptFeature{}}
	for _, id := range catalogueOrder(cat, ids) {
		was, now := effectiveState(from, id, p.Features), effectiveState(to, id, after.Features)
		switch {
		case (now.Source == SourceUser || now.Source == SourceLoaded) && now.Enabled == was.Enabled:
			rc.Kept = append(rc.Kept, KeptFeature{ID: id, Enabled: now.Enabled, Source: now.Source})
		case now.Enabled && !was.Enabled:
			rc.Enabled = append(rc.Enabled, id)
		case !now.Enabled && was.Enabled:
			rc.Disabled = append(rc.Disabled, id)
		}
	}
	return rc, nil
}

// catalogueOrder lists ids in the feature order of cat, followed by the ids cat does not know, sorted.
func catalogueOrder(cat model.Bundle, ids map[string]bool) []string {
	ordered := make([]string, 0, len(ids))
	for _, f := range cat.Features {
		if ids[f.ID] {
			ordered = append(ordered, f.ID)
		}
	}
	var rest []string
	for _, id := range sortedKeys(ids) {
		if !slices.Contains(ordered, id) {
			rest = append(rest, id)
		}
	}
	return append(ordered, rest...)
}
