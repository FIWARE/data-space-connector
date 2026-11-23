package profile

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

const (
	featIssuer       = "identity.issuer"
	featDID          = "identity.did"
	featVerification = "access.verification"
	featScorpio      = "data.scorpio"
	featRainbow      = "dsp.rainbow"
	featMarketLocal  = "market.local"
	featRegistry     = "ta.registry"
)

// taFeatures are the catalogue features of chart trust-anchor, sorted.
var taFeatures = []string{"consent.authority", "ta.onboarding", featRegistry}

// preset applies role to a fresh DSC profile, then the user toggles in sorted order.
func preset(t *testing.T, shipped []model.Bundle, role string, loaded, toggles map[string]bool) Profile {
	t.Helper()
	p, _, err := dscProfile().ApplyPreset(shipped, role, loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range sortedKeys(toggles) {
		if p, err = p.SetFeature(shipped, id, toggles[id]); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func userState(enabled bool) FeatureState { return FeatureState{Enabled: enabled, Source: SourceUser} }

func TestSetFeature(t *testing.T) {
	shipped := shippedBundles(t)
	tests := []struct {
		name    string
		base    Profile
		id      string
		enabled bool
	}{
		{"preset on to user off", preset(t, shipped, model.RoleProvider, nil, nil), featVerification, false},
		{"preset off to user on", preset(t, shipped, model.RoleProvider, nil, nil), featRainbow, true},
		{"loaded to user", preset(t, shipped, model.RoleProvider, map[string]bool{featScorpio: false}, nil), featScorpio, true},
		{"user back to the preset value stays user", preset(t, shipped, model.RoleProvider, nil, map[string]bool{featVerification: false}), featVerification, true},
		{"absent not-offered feature is added", preset(t, shipped, model.RoleConsumer, nil, nil), featMarketLocal, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := maps.Clone(tc.base.Features)
			got, err := tc.base.SetFeature(shipped, tc.id, tc.enabled)
			if err != nil {
				t.Fatal(err)
			}
			want := maps.Clone(before)
			want[tc.id] = userState(tc.enabled)
			if !maps.Equal(got.Features, want) {
				t.Errorf("features = %v, want %v", got.Features, want)
			}
			if !maps.Equal(tc.base.Features, before) {
				t.Errorf("input features changed to %v", tc.base.Features)
			}
			if errs := got.Validate(shipped); len(errs) != 0 {
				t.Errorf("Validate = %v, want no errors", errs)
			}
		})
	}
}

func TestSetFeatureRefuses(t *testing.T) {
	shipped := shippedBundles(t)
	provider := preset(t, shipped, model.RoleProvider, nil, nil)
	tests := []struct {
		name    string
		base    Profile
		id      string
		wantErr error
	}{
		{"no role", dscProfile(), featVerification, ErrNoRole},
		{"unknown feature", provider, "no.such", ErrUnknownFeature},
		{"provider and a trust-anchor feature", provider, featRegistry, ErrFeatureOfOtherChart},
		{"operator and a DSC feature", preset(t, shipped, model.RoleOperator, nil, nil), featVerification, ErrFeatureOfOtherChart},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.base.SetFeature(shipped, tc.id, true)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.base) {
				t.Errorf("profile = %+v, want it unchanged", got)
			}
		})
	}
}

func TestFeatureChoices(t *testing.T) {
	shipped := shippedBundles(t)
	consumer := preset(t, shipped, model.RoleConsumer, map[string]bool{featRainbow: true},
		map[string]bool{featIssuer: false, featMarketLocal: true, featDID: true})
	bare := preset(t, shipped, model.RoleProvider, nil, nil)
	bare.Features = nil
	tests := []struct {
		name string
		p    Profile
		want FeatureChoice
	}{
		{"recommended turned off by the user", consumer,
			FeatureChoice{ID: featIssuer, Offer: model.OfferRecommended, Preset: true, Enabled: false, Deliberate: true, Source: SourceUser}},
		{"not-offered turned on by the user", consumer,
			FeatureChoice{ID: featMarketLocal, Offer: model.OfferNotOffered, Enabled: true, Deliberate: true, Source: SourceUser}},
		{"available turned on by the values file", consumer,
			FeatureChoice{ID: featRainbow, Offer: model.OfferAvailable, Enabled: true, Deliberate: true, Source: SourceLoaded}},
		{"user choice equal to the preset", consumer,
			FeatureChoice{ID: featDID, Offer: model.OfferRecommended, Preset: true, Enabled: true, Source: SourceUser}},
		{"absent not-offered feature", consumer,
			FeatureChoice{ID: featScorpio, Offer: model.OfferNotOffered, Source: SourcePreset}},
		{"absent recommended feature", bare,
			FeatureChoice{ID: featVerification, Offer: model.OfferRecommended, Preset: true, Enabled: true, Source: SourcePreset}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			choices, err := tc.p.FeatureChoices(shipped)
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(choices, func(c FeatureChoice) bool { return c.ID == tc.want.ID })
			if i < 0 {
				t.Fatalf("no choice for %s in %v", tc.want.ID, choices)
			}
			if choices[i] != tc.want {
				t.Errorf("choice = %+v, want %+v", choices[i], tc.want)
			}
		})
	}
}

func TestFeatureChoicesScope(t *testing.T) {
	shipped := shippedBundles(t)
	cat, err := model.RoleCatalogue(shipped, dscChart, latest)
	if err != nil {
		t.Fatal(err)
	}
	var dscFeatures []string
	for _, f := range cat.Features {
		if !slices.Contains(taFeatures, f.ID) {
			dscFeatures = append(dscFeatures, f.ID)
		}
	}
	tests := []struct {
		role string
		want []string
	}{
		{model.RoleConsumer, dscFeatures},
		{model.RoleProvider, dscFeatures},
		{model.RoleCustom, dscFeatures},
		{model.RoleOperator, taFeatures},
	}
	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			choices, err := preset(t, shipped, tc.role, nil, nil).FeatureChoices(shipped)
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, c := range choices {
				ids = append(ids, c.ID)
			}
			// Catalogue order for the DSC chart; the trust-anchor list is compared as a set.
			if tc.role == model.RoleOperator {
				slices.Sort(ids)
			}
			if !slices.Equal(ids, tc.want) {
				t.Errorf("choices = %v, want %v", ids, tc.want)
			}
		})
	}
	if _, err := dscProfile().FeatureChoices(shipped); !errors.Is(err, ErrNoRole) {
		t.Errorf("without a role: err = %v, want %v", err, ErrNoRole)
	}
}

func TestValidateFeatureScopeAndSource(t *testing.T) {
	shipped := shippedBundles(t)
	tests := []struct {
		role    string
		id      string
		source  FeatureSource
		wantErr string
	}{
		{model.RoleConsumer, featScorpio, SourceUser, ""},
		{model.RoleConsumer, featScorpio, SourceLoaded, ""},
		{model.RoleConsumer, featScorpio, SourcePreset, "cannot come from the preset"},
		{model.RoleProvider, featRegistry, SourcePreset, "sets values of chart trust-anchor"},
		{model.RoleProvider, featRegistry, SourceLoaded, "sets values of chart trust-anchor"},
		{model.RoleProvider, featRegistry, SourceUser, "sets values of chart trust-anchor"},
		{model.RoleOperator, featVerification, SourceUser, "sets values of chart data-space-connector"},
	}
	for _, tc := range tests {
		t.Run(tc.role+"/"+tc.id+"/"+string(tc.source), func(t *testing.T) {
			p := preset(t, shipped, tc.role, nil, nil)
			p.Features = maps.Clone(p.Features)
			p.Features[tc.id] = FeatureState{Enabled: true, Source: tc.source}
			errs := p.Validate(shipped)
			if tc.wantErr == "" {
				if len(errs) != 0 {
					t.Errorf("Validate = %v, want no errors", errs)
				}
				return
			}
			if len(errs) != 1 || errs[0].Field != featuresFieldPrefix+tc.id || !strings.Contains(errs[0].Message, tc.wantErr) {
				t.Errorf("Validate = %v, want one error on %s containing %q", errs, tc.id, tc.wantErr)
			}
		})
	}
}

func TestApplyPresetCarriesUserToggles(t *testing.T) {
	shipped := shippedBundles(t)
	tests := []struct {
		name    string
		from    string
		toggles map[string]bool
		loaded  map[string]bool
		to      string
		// want lists entries the result must hold; absent lists ids it must not hold.
		want   map[string]FeatureState
		absent []string
		// wantDropped is nil when no chart switch is expected.
		wantDropped []string
	}{
		{name: "recommended off survives provider to consumer-provider", from: model.RoleProvider,
			toggles: map[string]bool{featVerification: false}, to: model.RoleConsumerProvider,
			want: map[string]FeatureState{featVerification: userState(false)}},
		{name: "available on survives provider to consumer, where it is not offered", from: model.RoleProvider,
			toggles: map[string]bool{featMarketLocal: true}, to: model.RoleConsumer,
			want: map[string]FeatureState{featMarketLocal: userState(true)}},
		{name: "user toggle beats a loaded toggle", from: model.RoleProvider,
			toggles: map[string]bool{featScorpio: true}, loaded: map[string]bool{featScorpio: false}, to: model.RoleProvider,
			want: map[string]FeatureState{featScorpio: userState(true)}},
		{name: "chart switch names the dropped user toggles", from: model.RoleProvider,
			toggles: map[string]bool{featVerification: false, featMarketLocal: true}, to: model.RoleOperator,
			absent: []string{featVerification, featMarketLocal}, wantDropped: []string{featVerification, featMarketLocal}},
		{name: "chart switch without user toggles drops nothing", from: model.RoleProvider, to: model.RoleOperator,
			wantDropped: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := preset(t, shipped, tc.from, nil, tc.toggles)
			got, sw, err := p.ApplyPreset(shipped, tc.to, tc.loaded)
			if err != nil {
				t.Fatal(err)
			}
			for id, want := range tc.want {
				if f, ok := got.Features[id]; !ok || f != want {
					t.Errorf("%s = %+v (present %t), want %+v", id, f, ok, want)
				}
			}
			for _, id := range tc.absent {
				if f, ok := got.Features[id]; ok {
					t.Errorf("%s = %+v, want it absent", id, f)
				}
			}
			switch {
			case tc.wantDropped == nil && sw != nil:
				t.Errorf("switch = %+v, want none", sw)
			case tc.wantDropped != nil && (sw == nil || sw.DroppedChoices == nil):
				t.Errorf("switch = %+v, want droppedChoices %v", sw, tc.wantDropped)
			case tc.wantDropped != nil && !slices.Equal(sw.DroppedChoices, slices.Sorted(slices.Values(tc.wantDropped))):
				t.Errorf("droppedChoices = %v, want %v sorted", sw.DroppedChoices, tc.wantDropped)
			}
			if errs := got.Validate(shipped); len(errs) != 0 {
				t.Errorf("Validate = %v, want no errors", errs)
			}
		})
	}
}

func TestUserChoicesRoundTrip(t *testing.T) {
	shipped := shippedBundles(t)
	p := preset(t, shipped, model.RoleConsumer, nil, map[string]bool{featIssuer: false, featMarketLocal: true})
	data, err := Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if errs := got.Validate(shipped); len(errs) != 0 {
		t.Fatalf("Validate = %v, want no errors", errs)
	}
	for id, want := range map[string]FeatureState{featIssuer: userState(false), featMarketLocal: userState(true)} {
		if f := got.Features[id]; f != want {
			t.Errorf("%s = %+v, want %+v", id, f, want)
		}
	}
}
