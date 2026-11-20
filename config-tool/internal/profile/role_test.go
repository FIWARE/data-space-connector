package profile

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	roleGoldenFile = "testdata/role.dscprofile.yaml"
	// shippedTAVersion and shippedDSCRevision are the shipped trust-anchor bundle and DSC bundle revision.
	shippedTAVersion   = "1.3.6"
	shippedDSCRevision = 2
)

// Feature ids of the shipped catalogue, grouped as in the preset table of IMPLEMENTATION_PLAN.md decision 5.
var (
	identityCore    = []string{"identity.issuer", "identity.did"}
	identityExtra   = []string{"identity.statuslist", "identity.vcoperator", "identity.wallets", "identity.eidas"}
	accessFeatures  = []string{"access.verification", "access.policies"}
	dspFeatures     = []string{"dsp.edc", "dsp.rainbow", "dsp.config"}
	obsFeatures     = []string{"obs.tracing", "obs.traceStore", "obs.metrics"}
	providerOptions = []string{"data.external", "contract.tmforum", "contract.management", "market.local", "market.central", "consent.provider"}
)

func shippedBundles(t *testing.T) []model.Bundle {
	t.Helper()
	b, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// dscProfile is a provider-less profile on the shipped DSC bundle, normalised as saveModel leaves it.
func dscProfile() Profile {
	return Profile{
		Format: FormatVersion,
		Model:  ModelRef{Chart: dscChart, Version: latest, Revision: shippedDSCRevision},
		Target: validTarget(),
	}
}

// features builds a Features map: recommended ids enabled, available ids disabled, all from the preset.
func features(recommended, available []string) map[string]FeatureState {
	m := map[string]FeatureState{}
	for _, id := range recommended {
		m[id] = FeatureState{Enabled: true, Source: SourcePreset}
	}
	for _, id := range available {
		m[id] = FeatureState{Enabled: false, Source: SourcePreset}
	}
	return m
}

func concat(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

func TestApplyPresetRoles(t *testing.T) {
	providerFeatures := features(
		concat(identityCore, accessFeatures, []string{"data.scorpio"}),
		concat(identityExtra, providerOptions, dspFeatures, obsFeatures),
	)
	tests := []struct {
		role string
		want map[string]FeatureState
	}{
		{model.RoleConsumer, features(identityCore, concat(identityExtra, dspFeatures, obsFeatures))},
		{model.RoleProvider, providerFeatures},
		{model.RoleConsumerProvider, providerFeatures},
		{model.RoleOperator, features([]string{"ta.registry"}, []string{"consent.authority", "ta.onboarding"})},
		{model.RoleCustom, features(nil, concat(identityCore, identityExtra, accessFeatures, []string{"data.scorpio"}, providerOptions, dspFeatures, obsFeatures))},
	}
	shipped := shippedBundles(t)
	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			got, _, err := dscProfile().ApplyPreset(shipped, tc.role, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Role != tc.role {
				t.Errorf("role = %q, want %q", got.Role, tc.role)
			}
			if !maps.Equal(got.Features, tc.want) {
				t.Errorf("features = %v, want %v", got.Features, tc.want)
			}
			if errs := got.Validate(shipped); len(errs) != 0 {
				t.Errorf("Validate = %v, want no errors", errs)
			}
		})
	}
}

func TestApplyPresetChartSwitch(t *testing.T) {
	shipped := shippedBundles(t)
	dscCatalogue := &ModelRef{Chart: dscChart, Version: latest, Revision: shippedDSCRevision}

	op, sw, err := dscProfile().ApplyPreset(shipped, model.RoleOperator, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := (ChartSwitch{FromChart: dscChart, FromVersion: latest, ToChart: taChart}); sw == nil || *sw != want {
		t.Errorf("switch = %+v, want %+v", sw, want)
	}
	if op.Target.Chart != taChart || op.Target.ChartVersion != "" {
		t.Errorf("target = %s@%q, want %s with an empty version", op.Target.Chart, op.Target.ChartVersion, taChart)
	}
	if want := (ModelRef{Chart: taChart, Version: shippedTAVersion, Revision: 1}); op.Model != want {
		t.Errorf("model = %+v, want %+v", op.Model, want)
	}
	if !reflect.DeepEqual(op.Catalogue, dscCatalogue) {
		t.Errorf("catalogue = %+v, want %+v", op.Catalogue, dscCatalogue)
	}

	back, sw, err := op.ApplyPreset(shipped, model.RoleProvider, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := (ChartSwitch{FromChart: taChart, ToChart: dscChart}); sw == nil || *sw != want {
		t.Errorf("switch back = %+v, want %+v", sw, want)
	}
	if back.Target.Chart != dscChart || back.Target.ChartVersion != "" || back.Catalogue != nil {
		t.Errorf("back = %s@%q catalogue %+v, want %s, an empty version and no catalogue", back.Target.Chart, back.Target.ChartVersion, back.Catalogue, dscChart)
	}

	same, sw, err := dscProfile().ApplyPreset(shipped, model.RoleConsumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sw != nil || same.Target.ChartVersion != latest || same.Catalogue != nil {
		t.Errorf("no switch: switch %+v, version %q, catalogue %+v; want nil, %s, nil", sw, same.Target.ChartVersion, same.Catalogue, latest)
	}
}

func TestApplyPresetWithSource(t *testing.T) {
	shipped := shippedBundles(t)
	p := dscProfile()
	p.Source = &Source{File: "values.yaml", SHA256: sourceHash}
	tests := []struct {
		role    string
		wantErr error
	}{
		{model.RoleOperator, ErrChartSwitchLoaded},
		{model.RoleProvider, nil},
		{model.RoleCustom, nil},
	}
	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			got, _, err := p.ApplyPreset(shipped, tc.role, nil)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && !reflect.DeepEqual(got, p) {
				t.Errorf("profile = %+v, want it unchanged", got)
			}
		})
	}
}

func TestApplyPresetLoadedToggles(t *testing.T) {
	shipped := shippedBundles(t)
	provider, _, err := dscProfile().ApplyPreset(shipped, model.RoleProvider, nil)
	if err != nil {
		t.Fatal(err)
	}
	consumer, _, err := dscProfile().ApplyPreset(shipped, model.RoleConsumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	with := func(base map[string]FeatureState, id string, s *FeatureState) map[string]FeatureState {
		m := maps.Clone(base)
		if s == nil {
			delete(m, id)
		} else {
			m[id] = *s
		}
		return m
	}
	tests := []struct {
		name    string
		role    string
		loaded  map[string]bool
		want    map[string]FeatureState
		wantErr string
	}{
		{name: "loaded false beats a recommendation", role: model.RoleProvider, loaded: map[string]bool{"data.scorpio": false},
			want: with(provider.Features, "data.scorpio", &FeatureState{Enabled: false, Source: SourceLoaded})},
		{name: "loaded true on an available feature", role: model.RoleConsumer, loaded: map[string]bool{"dsp.rainbow": true},
			want: with(consumer.Features, "dsp.rainbow", &FeatureState{Enabled: true, Source: SourceLoaded})},
		{name: "loaded false on a not-offered feature is omitted", role: model.RoleConsumer, loaded: map[string]bool{"data.scorpio": false},
			want: consumer.Features},
		{name: "loaded true on a not-offered feature", role: model.RoleConsumer, loaded: map[string]bool{"data.scorpio": true, "access.policies": true},
			wantErr: "access.policies, data.scorpio"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := dscProfile()
			got, _, err := p.ApplyPreset(shipped, tc.role, tc.loaded)
			if tc.wantErr != "" {
				if !errors.Is(err, ErrPresetConflictsLoaded) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %v naming %s", err, ErrPresetConflictsLoaded, tc.wantErr)
				}
				if !reflect.DeepEqual(got, p) {
					t.Errorf("profile = %+v, want it unchanged", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(got.Features, tc.want) {
				t.Errorf("features = %v, want %v", got.Features, tc.want)
			}
			if errs := got.Validate(shipped); len(errs) != 0 {
				t.Errorf("Validate = %v, want no errors", errs)
			}
		})
	}
}

func TestApplyPresetRefuses(t *testing.T) {
	shipped := shippedBundles(t)
	tests := []struct {
		name   string
		role   string
		loaded map[string]bool
	}{
		{"unknown role", "admin", nil},
		{"loaded toggle not in the catalogue", model.RoleProvider, map[string]bool{"no.such": true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := dscProfile()
			got, sw, err := p.ApplyPreset(shipped, tc.role, tc.loaded)
			if err == nil || sw != nil || !reflect.DeepEqual(got, p) {
				t.Errorf("ApplyPreset = %+v, %+v, %v; want the profile unchanged and an error", got, sw, err)
			}
		})
	}
}

func TestApplyPresetDoesNotMutateInput(t *testing.T) {
	shipped := shippedBundles(t)
	p := dscProfile()
	p.Features = map[string]FeatureState{"dsp.edc": {Enabled: true, Source: SourceUser}}
	p.Catalogue = &ModelRef{Chart: dscChart, Version: latest, Revision: shippedDSCRevision}
	loaded := map[string]bool{"dsp.rainbow": true}
	wantFeatures, wantLoaded, wantCatalogue := maps.Clone(p.Features), maps.Clone(loaded), *p.Catalogue

	if _, _, err := p.ApplyPreset(shipped, model.RoleOperator, loaded); err == nil {
		t.Fatal("operator with a dsp toggle loaded: want an error")
	}
	if _, _, err := p.ApplyPreset(shipped, model.RoleProvider, loaded); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(p.Features, wantFeatures) || !maps.Equal(loaded, wantLoaded) || *p.Catalogue != wantCatalogue {
		t.Errorf("input changed: features %v, loaded %v, catalogue %+v", p.Features, loaded, *p.Catalogue)
	}
}

// TestRoleGolden pins the encoding of an Operator profile and its round trip.
func TestRoleGolden(t *testing.T) {
	shipped := shippedBundles(t)
	op, _, err := dscProfile().ApplyPreset(shipped, model.RoleOperator, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(roleGoldenFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Marshal =\n%s\nwant\n%s", got, want)
	}
	back, err := Unmarshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if back.Role != op.Role || !maps.Equal(back.Features, op.Features) || !reflect.DeepEqual(back.Catalogue, op.Catalogue) {
		t.Errorf("Unmarshal = %+v, want role, features and catalogue of %+v", back, op)
	}
	if errs := back.Validate(shipped); len(errs) != 0 {
		t.Errorf("Validate = %v, want no errors", errs)
	}
}

func TestRevisionNotes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Profile)
		want   []string
	}{
		{"shipped revisions", func(p *Profile) { p.Model.Revision = shippedRev }, nil},
		{"older model", func(*Profile) {}, []string{"the profile was saved with revision 1 of bundle data-space-connector@10.16.0; this build ships revision 2"}},
		{"older catalogue", func(p *Profile) {
			p.Model = ModelRef{Chart: taChart, Version: taVersion, Revision: 1}
			p.Catalogue = &ModelRef{Chart: dscChart, Version: latest, Revision: 1}
		}, []string{"the profile was saved with revision 1 of bundle data-space-connector@10.16.0; this build ships revision 2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := fullProfile()
			tc.mutate(&p)
			if got := p.RevisionNotes(testBundles); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RevisionNotes = %q, want %q", got, tc.want)
			}
		})
	}
}

// twoRoleBundles is testBundles with the older DSC bundle also declaring roles, under a different provider preset.
func twoRoleBundles() []model.Bundle {
	b := slices.Clone(testBundles)
	b[1].Features = b[0].Features
	b[1].Roles = []model.Role{
		{ID: model.RoleProvider, Chart: dscChart, Features: map[string]model.Offer{dscFeature: model.OfferRecommended, taFeature: model.OfferNotOffered}},
		{ID: model.RoleOperator, Chart: taChart, Features: map[string]model.Offer{dscFeature: model.OfferNotOffered, taFeature: model.OfferAvailable}},
	}
	return b
}

func TestCatalogueBundle(t *testing.T) {
	b := twoRoleBundles()
	dscLatest := ModelRef{Chart: dscChart, Version: latest, Revision: shippedRev}
	dscOlder := ModelRef{Chart: dscChart, Version: older, Revision: 1}
	ta := ModelRef{Chart: taChart, Version: taVersion, Revision: 1}
	tests := []struct {
		name  string
		model ModelRef
		held  *ModelRef
		want  string
	}{
		{"model declares roles, nothing held", dscLatest, nil, latest},
		{"model declares roles, another catalogue held", dscLatest, &dscOlder, latest},
		{"fallback, nothing held", ta, nil, latest},
		{"fallback, older catalogue held", ta, &dscOlder, older},
		{"fallback, held bundle of another chart", ta, &ta, latest},
		{"fallback, held bundle not shipped", ta, &ModelRef{Chart: dscChart, Version: "9.9.9", Revision: 1}, latest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CatalogueBundle(b, tc.model, tc.held)
			if err != nil || got.Chart != dscChart || got.Version != tc.want {
				t.Errorf("CatalogueBundle = %s@%s, %v; want %s@%s", got.Chart, got.Version, err, dscChart, tc.want)
			}
		})
	}
}

// TestApplyPresetIgnoresHeldCatalogueOfRoleModel reads presets from the model's own bundle, as a save records them.
func TestApplyPresetIgnoresHeldCatalogueOfRoleModel(t *testing.T) {
	b := twoRoleBundles()
	p := Profile{
		Format:    FormatVersion,
		Model:     ModelRef{Chart: dscChart, Version: latest, Revision: shippedRev},
		Target:    validTarget(),
		Catalogue: &ModelRef{Chart: dscChart, Version: older, Revision: 1},
	}
	got, sw, err := p.ApplyPreset(b, model.RoleProvider, nil)
	if err != nil || sw != nil {
		t.Fatalf("ApplyPreset: %+v, %v", sw, err)
	}
	want := map[string]FeatureState{dscFeature: {Enabled: false, Source: SourcePreset}}
	if got.Catalogue != nil || !maps.Equal(got.Features, want) {
		t.Errorf("catalogue %+v, features %v; want nil and %v", got.Catalogue, got.Features, want)
	}
	if errs := got.Validate(b); len(errs) > 0 {
		t.Errorf("Validate: %v", errs)
	}
}
