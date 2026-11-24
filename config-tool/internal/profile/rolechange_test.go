package profile

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

const (
	featAuthority = "consent.authority"
	// Ids no catalogue knows, as a profile of an older bundle revision could record them.
	featGoneA = "aa.gone"
	featGoneZ = "zz.gone"
)

func idSet(lists ...[]string) map[string]bool {
	s := map[string]bool{}
	for _, l := range lists {
		for _, id := range l {
			s[id] = true
		}
	}
	return s
}

// minus is a without the ids of every set in drop.
func minus(a map[string]bool, drop ...map[string]bool) map[string]bool {
	out := maps.Clone(a)
	maps.DeleteFunc(out, func(id string, _ bool) bool {
		return slices.ContainsFunc(drop, func(d map[string]bool) bool { return d[id] })
	})
	return out
}

func TestRoleChangeTo(t *testing.T) {
	shipped := shippedBundles(t)
	tests := []struct {
		name     string
		from, to string
		loaded   map[string]bool
		// reload, when set, replaces loaded for the change, as a values file loaded after the first pick does.
		reload   map[string]bool
		toggles  map[string]bool
		edit     func(*Profile)
		wantKept []KeptFeature
		// Enabled and Disabled are the preset delta of from and to without the kept ids, without
		// notChanged and with alsoEnabled and alsoDisabled.
		notChanged   []string
		alsoEnabled  []string
		alsoDisabled []string
	}{
		{name: "consumer to consumer-provider", from: model.RoleConsumer, to: model.RoleConsumerProvider},
		{name: "consumer-provider to consumer", from: model.RoleConsumerProvider, to: model.RoleConsumer},
		{name: "hand toggles survive consumer to consumer-provider", from: model.RoleConsumer, to: model.RoleConsumerProvider,
			toggles:  map[string]bool{featScorpio: false, featRainbow: true},
			wantKept: []KeptFeature{{featScorpio, false, SourceUser}, {featRainbow, true, SourceUser}}},
		{name: "hand toggles survive consumer-provider to consumer", from: model.RoleConsumerProvider, to: model.RoleConsumer,
			toggles:  map[string]bool{featScorpio: true, featIssuer: false},
			wantKept: []KeptFeature{{featScorpio, true, SourceUser}, {featIssuer, false, SourceUser}}},
		{name: "loaded toggle is kept with its source", from: model.RoleConsumerProvider, to: model.RoleConsumer,
			loaded:   map[string]bool{featRainbow: true},
			wantKept: []KeptFeature{{featRainbow, true, SourceLoaded}}},
		{name: "loaded toggle that changes state is not kept", from: model.RoleConsumerProvider, to: model.RoleConsumer,
			reload: map[string]bool{featRainbow: true}, alsoEnabled: []string{featRainbow}},
		{name: "unrecorded recommended feature counts as on before", from: model.RoleConsumer, to: model.RoleConsumerProvider,
			edit: func(p *Profile) { delete(p.Features, featIssuer) }},
		{name: "unrecorded recommended feature goes off on a switch", from: model.RoleConsumer, to: model.RoleOperator,
			edit: func(p *Profile) { delete(p.Features, featIssuer) }},
		{name: "consumer to operator", from: model.RoleConsumer, to: model.RoleOperator},
		{name: "operator to consumer", from: model.RoleOperator, to: model.RoleConsumer},
		{name: "switch keeps an off in-scope user toggle", from: model.RoleConsumer, to: model.RoleOperator,
			// SetFeature refuses a feature of the other chart, but an off record of one is valid.
			edit:     func(p *Profile) { p.Features[featAuthority] = userState(false) },
			wantKept: []KeptFeature{{featAuthority, false, SourceUser}}},
		{name: "switch drops toggles of the old chart", from: model.RoleConsumer, to: model.RoleOperator,
			toggles:    map[string]bool{featRainbow: true, featIssuer: false},
			notChanged: []string{featIssuer}, alsoDisabled: []string{featRainbow}},
		{name: "same role re-applied", from: model.RoleConsumer, to: model.RoleConsumer},
		{name: "ids outside the catalogue sort after it", from: model.RoleConsumer, to: model.RoleConsumer,
			edit: func(p *Profile) {
				p.Features[featGoneZ], p.Features[featGoneA] = userState(true), userState(true)
			},
			alsoDisabled: []string{featGoneA, featGoneZ}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := preset(t, shipped, tc.from, tc.loaded, tc.toggles)
			if tc.edit != nil {
				tc.edit(&p)
			}
			loaded := tc.loaded
			if tc.reload != nil {
				loaded = tc.reload
			}
			after, sw, err := p.ApplyPreset(shipped, tc.to, loaded)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.RoleChangeTo(shipped, after)
			if err != nil {
				t.Fatal(err)
			}
			if got.FromRole != tc.from || got.ToRole != tc.to {
				t.Errorf("roles = %s -> %s, want %s -> %s", got.FromRole, got.ToRole, tc.from, tc.to)
			}

			kept, wantKept := map[string]KeptFeature{}, map[string]KeptFeature{}
			for _, k := range got.Kept {
				kept[k.ID] = k
			}
			for _, k := range tc.wantKept {
				wantKept[k.ID] = k
			}
			if !maps.Equal(kept, wantKept) {
				t.Errorf("kept = %v, want %v", got.Kept, tc.wantKept)
			}
			recFrom := recommended(t, shipped, dscChart, tc.from)
			recTo := recommended(t, shipped, dscChart, tc.to)
			skip := idSet(slices.Collect(maps.Keys(wantKept)), tc.notChanged)
			wantEnabled := minus(recTo, recFrom, skip)
			wantDisabled := minus(recFrom, recTo, skip)
			maps.Copy(wantEnabled, idSet(tc.alsoEnabled))
			maps.Copy(wantDisabled, idSet(tc.alsoDisabled))
			if g := idSet(got.Enabled); !maps.Equal(g, wantEnabled) {
				t.Errorf("enabled = %v, want %v", got.Enabled, slices.Sorted(maps.Keys(wantEnabled)))
			}
			if g := idSet(got.Disabled); !maps.Equal(g, wantDisabled) {
				t.Errorf("disabled = %v, want %v", got.Disabled, slices.Sorted(maps.Keys(wantDisabled)))
			}
			checkRoleChange(t, shipped, p, recFrom, after, got, sw)
		})
	}
}

// checkRoleChange asserts the invariants every summary holds, whatever the roles.
func checkRoleChange(t *testing.T, shipped []model.Bundle, before Profile, recFrom map[string]bool, after Profile, rc RoleChange, sw *ChartSwitch) {
	t.Helper()
	if rc.Enabled == nil || rc.Disabled == nil || rc.Kept == nil {
		t.Errorf("summary %+v has a nil list", rc)
	}
	keptIDs := make([]string, 0, len(rc.Kept))
	for _, k := range rc.Kept {
		keptIDs = append(keptIDs, k.ID)
		was, ok := before.Features[k.ID]
		if !ok {
			was.Enabled = recFrom[k.ID]
		}
		if now := after.Features[k.ID]; k.Enabled != was.Enabled || now.Enabled != k.Enabled || now.Source != k.Source {
			t.Errorf("kept %+v: before %+v, after %+v; want the same state", k, was, now)
		}
	}
	if n := len(idSet(rc.Enabled, rc.Disabled, keptIDs)); n != len(rc.Enabled)+len(rc.Disabled)+len(keptIDs) {
		t.Errorf("enabled %v, disabled %v and kept %v overlap", rc.Enabled, rc.Disabled, keptIDs)
	}

	cat, err := CatalogueBundle(shipped, after.Model, after.Catalogue)
	if err != nil {
		t.Fatal(err)
	}
	pos := func(id string) int {
		if i := slices.IndexFunc(cat.Features, func(f model.Feature) bool { return f.ID == id }); i >= 0 {
			return i
		}
		return len(cat.Features)
	}
	order := func(a, b string) int { return cmp.Or(cmp.Compare(pos(a), pos(b)), strings.Compare(a, b)) }
	for _, l := range [][]string{rc.Enabled, rc.Disabled, keptIDs} {
		if !slices.IsSortedFunc(l, order) {
			t.Errorf("%v is not in catalogue order", l)
		}
	}

	if sw == nil {
		return
	}
	for _, id := range sw.DroppedChoices {
		wantDisabled := before.Features[id].Enabled
		if slices.Contains(keptIDs, id) || slices.Contains(rc.Enabled, id) || slices.Contains(rc.Disabled, id) != wantDisabled {
			t.Errorf("dropped %s (on before: %t): enabled %v, disabled %v, kept %v", id, wantDisabled, rc.Enabled, rc.Disabled, keptIDs)
		}
	}
}

func TestRoleChangeToWithoutRole(t *testing.T) {
	shipped := shippedBundles(t)
	after := preset(t, shipped, model.RoleConsumer, nil, nil)
	if _, err := dscProfile().RoleChangeTo(shipped, after); !errors.Is(err, ErrNoRole) {
		t.Errorf("err = %v, want %v", err, ErrNoRole)
	}
}
