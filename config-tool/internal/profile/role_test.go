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
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	roleGoldenFile = "testdata/role.dscprofile.yaml"
	// shippedTAVersion, shippedTARevision and shippedDSCRevision identify the shipped bundles.
	shippedTAVersion   = "1.3.6"
	shippedTARevision  = 2
	shippedDSCRevision = 15
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
	if want := (ChartSwitch{FromChart: dscChart, FromVersion: latest, ToChart: taChart, DroppedChoices: []string{}, CarriedAnswers: []string{}, DroppedAnswers: []string{}}); sw == nil || !reflect.DeepEqual(*sw, want) {
		t.Errorf("switch = %+v, want %+v", sw, want)
	}
	if op.Target.Chart != taChart || op.Target.ChartVersion != "" {
		t.Errorf("target = %s@%q, want %s with an empty version", op.Target.Chart, op.Target.ChartVersion, taChart)
	}
	if want := (ModelRef{Chart: taChart, Version: shippedTAVersion, Revision: shippedTARevision}); op.Model != want {
		t.Errorf("model = %+v, want %+v", op.Model, want)
	}
	if !reflect.DeepEqual(op.Catalogue, dscCatalogue) {
		t.Errorf("catalogue = %+v, want %+v", op.Catalogue, dscCatalogue)
	}

	back, sw, err := op.ApplyPreset(shipped, model.RoleProvider, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := (ChartSwitch{FromChart: taChart, ToChart: dscChart, DroppedChoices: []string{}, CarriedAnswers: []string{}, DroppedAnswers: []string{}}); sw == nil || !reflect.DeepEqual(*sw, want) {
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
	loadedOn := FeatureState{Enabled: true, Source: SourceLoaded}
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
			want: with(consumer.Features, "dsp.rainbow", &loadedOn)},
		{name: "loaded false on a not-offered feature is omitted", role: model.RoleConsumer, loaded: map[string]bool{"data.scorpio": false},
			want: consumer.Features},
		{name: "loaded true on a not-offered feature is kept as loaded", role: model.RoleConsumer, loaded: map[string]bool{"data.scorpio": true, "access.policies": true},
			want: with(with(consumer.Features, "data.scorpio", &loadedOn), "access.policies", &loadedOn)},
		{name: "loaded true on a feature of the other chart", role: model.RoleProvider, loaded: map[string]bool{"ta.registry": true, "consent.authority": true},
			wantErr: "consent.authority, ta.registry"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := dscProfile()
			got, _, err := p.ApplyPreset(shipped, tc.role, tc.loaded)
			if tc.wantErr != "" {
				if !errors.Is(err, ErrFeatureOfOtherChart) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %v naming %s", err, ErrFeatureOfOtherChart, tc.wantErr)
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
	p.Answers = map[string]any{answerConfig: true}
	loaded := map[string]bool{"dsp.rainbow": true}
	wantFeatures, wantLoaded, wantCatalogue, wantAnswers := maps.Clone(p.Features), maps.Clone(loaded), *p.Catalogue, maps.Clone(p.Answers)

	if _, _, err := p.ApplyPreset(shipped, model.RoleOperator, loaded); err == nil {
		t.Fatal("operator with a dsp toggle loaded: want an error")
	}
	for _, role := range []string{model.RoleProvider, model.RoleOperator} {
		got, _, err := p.ApplyPreset(shipped, role, map[string]bool{})
		if err != nil {
			t.Fatal(err)
		}
		got.Answers["written.after"] = 1
	}
	if _, _, err := p.ApplyPreset(shipped, model.RoleProvider, loaded); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(p.Features, wantFeatures) || !maps.Equal(loaded, wantLoaded) || *p.Catalogue != wantCatalogue || !maps.Equal(p.Answers, wantAnswers) {
		t.Errorf("input changed: features %v, loaded %v, catalogue %+v, answers %v", p.Features, loaded, *p.Catalogue, p.Answers)
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

// TestValidateCatalogueAgreesWithCatalogueBundle opens only a catalogue that a save would also record.
func TestValidateCatalogueAgreesWithCatalogueBundle(t *testing.T) {
	b := twoRoleBundles()
	dscLatest := ModelRef{Chart: dscChart, Version: latest, Revision: shippedRev}
	dscOlder := ModelRef{Chart: dscChart, Version: older, Revision: 1}
	ta := ModelRef{Chart: taChart, Version: taVersion, Revision: 1}
	provider := Profile{Format: FormatVersion, Model: dscLatest, Target: validTarget(), Role: model.RoleProvider,
		Features: map[string]FeatureState{dscFeature: {Enabled: true, Source: SourcePreset}}}
	operator := Profile{Format: FormatVersion, Model: ta, Target: Target{Chart: taChart}, Role: model.RoleOperator,
		Features: map[string]FeatureState{taFeature: {Enabled: true, Source: SourcePreset}}}
	tests := []struct {
		name      string
		p         Profile
		catalogue *ModelRef
		want      []string
	}{
		{"role model, no catalogue", provider, nil, nil},
		{"role model, older catalogue recorded", provider, &dscOlder, []string{FieldCatalogue}},
		{"role model, own bundle recorded", provider, &dscLatest, []string{FieldCatalogue}},
		{"fallback, latest catalogue", operator, &dscLatest, nil},
		{"fallback, older catalogue kept", operator, &dscOlder, nil},
		{"fallback, no catalogue", operator, nil, []string{FieldRole}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.p
			p.Catalogue = tc.catalogue
			var got []string
			for _, e := range p.Validate(b) {
				got = append(got, e.Field)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate fields = %v, want %v (%v)", got, tc.want, p.Validate(b))
			}
		})
	}
}

// Answer ids of the fixture rules answerBundles adds; answerUnknown has no rule in any bundle.
const (
	answerTarget      = "target.releaseLabel"
	answerFoundations = "foundations.organization"
	answerConfig      = "config.elsi"
	answerUnknown     = "no.rule"
)

// Recognize rules answerBundles adds, because the shipped bundles have none of stage target or foundations.
const (
	dscAnswerRules = `  - {path: fullnameOverride, answer: target.releaseLabel, stage: target, kind: value}
  - {path: nameOverride, answer: foundations.organization, stage: foundations, kind: value}
  - {path: elsi.enabled, answer: config.elsi, stage: configuration, kind: toggle}
`
	taAnswerRules = `  - {path: managedPostgres.teamId, answer: foundations.organization, stage: foundations, kind: value}
`
	recognizeKey = "recognize:\n"
	// answerFoundation declares answerFoundations in both bundles, so a chart switch carries it either way.
	answerFoundation = "  - {id: foundations.organization, stage: foundations, type: string, label: Organization}\n"
	foundationsKey   = "\nfoundations:\n"
)

// answerBundles are the shipped bundles with recognize rules for answers of several stages and a foundation added.
func answerBundles(t *testing.T) []model.Bundle {
	t.Helper()
	read := func(name string) string {
		b, err := bundles.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	dsc, ta := read(latest+"/bundle.yaml"), read(shippedTAVersion+"/bundle.yaml")
	if !strings.Contains(dsc, recognizeKey) || strings.Contains(ta, recognizeKey) ||
		strings.Count(dsc, foundationsKey) != 1 || strings.Count(ta, foundationsKey) != 1 {
		t.Fatal("shipped bundles changed shape: want recognize rules in the DSC bundle only and foundations in both")
	}
	dsc = strings.Replace(dsc, recognizeKey, recognizeKey+dscAnswerRules, 1)
	fsys := fstest.MapFS{
		latest + "/bundle.yaml":           {Data: []byte(strings.Replace(dsc, foundationsKey, foundationsKey+answerFoundation, 1))},
		shippedTAVersion + "/bundle.yaml": {Data: []byte(strings.Replace(ta, foundationsKey, foundationsKey+answerFoundation, 1) + recognizeKey + taAnswerRules)},
	}
	b, err := model.Versions(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// recommended are the ids role recommends in the latest bundle of chart.
func recommended(t *testing.T, shipped []model.Bundle, chart, role string) map[string]bool {
	t.Helper()
	i := slices.IndexFunc(shipped, func(b model.Bundle) bool { return b.Chart == chart && b.Latest })
	r := findRole(shipped[i], role)
	if r == nil {
		t.Fatalf("role %s is not in bundle %s", role, chart)
	}
	ids := map[string]bool{}
	for id, o := range r.Features {
		if o == model.OfferRecommended {
			ids[id] = true
		}
	}
	return ids
}

func enabledIDs(p Profile) map[string]bool {
	ids := map[string]bool{}
	for id, f := range p.Features {
		if f.Enabled {
			ids[id] = true
		}
	}
	return ids
}

func TestApplyPresetConsumerToConsumerProviderGains(t *testing.T) {
	shipped := shippedBundles(t)
	consumer := preset(t, shipped, model.RoleConsumer, nil, nil)
	both, _, err := consumer.ApplyPreset(shipped, model.RoleConsumerProvider, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := recommended(t, shipped, dscChart, model.RoleConsumerProvider)
	maps.DeleteFunc(want, func(id string, _ bool) bool { return recommended(t, shipped, dscChart, model.RoleConsumer)[id] })
	if len(want) == 0 {
		t.Fatal("consumer-provider recommends nothing beyond consumer; the test proves nothing")
	}
	got := enabledIDs(both)
	maps.DeleteFunc(got, func(id string, _ bool) bool { return enabledIDs(consumer)[id] })
	if !maps.Equal(got, want) {
		t.Errorf("gained %v, want %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
	}
}

func TestApplyPresetKeepsAnswersWithinChart(t *testing.T) {
	shipped := shippedBundles(t)
	answers := map[string]any{"data.scorpio.endpoint": "http://scorpio:9090", "access.policies.count": 3, "market.local.enabled": true}
	tests := []struct {
		name, from, to string
		// wentOff is a feature on before and off after, proving answers outlive it; "" when none must.
		wentOff string
	}{
		{name: "features stay on", from: model.RoleProvider, to: model.RoleConsumerProvider},
		{name: "features go off", from: model.RoleProvider, to: model.RoleConsumer, wentOff: featScorpio},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := preset(t, shipped, tc.from, nil, nil)
			p.Answers = maps.Clone(answers)
			want, err := Marshal(Profile{Answers: p.Answers})
			if err != nil {
				t.Fatal(err)
			}
			got, sw, err := p.ApplyPreset(shipped, tc.to, nil)
			if err != nil || sw != nil {
				t.Fatalf("ApplyPreset = %+v, %v; want no switch and no error", sw, err)
			}
			if tc.wentOff != "" && (!p.Features[tc.wentOff].Enabled || got.Features[tc.wentOff].Enabled) {
				t.Errorf("%s before %+v, after %+v; want on, then off", tc.wentOff, p.Features[tc.wentOff], got.Features[tc.wentOff])
			}
			after, err := Marshal(Profile{Answers: got.Answers})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, want) {
				t.Errorf("answers =\n%s\nwant\n%s", after, want)
			}
		})
	}
}

func TestApplyPresetKeepsHandTogglesBothWays(t *testing.T) {
	shipped := shippedBundles(t)
	if recommended(t, shipped, dscChart, model.RoleConsumerProvider)[featRainbow] || !recommended(t, shipped, dscChart, model.RoleConsumerProvider)[featIssuer] {
		t.Fatalf("want %s not recommended and %s recommended by %s", featRainbow, featIssuer, model.RoleConsumerProvider)
	}
	want := map[string]FeatureState{featRainbow: userState(true), featIssuer: userState(false)}
	p := preset(t, shipped, model.RoleConsumer, nil, map[string]bool{featRainbow: true, featIssuer: false})
	for _, role := range []string{model.RoleConsumerProvider, model.RoleConsumer} {
		var err error
		if p, _, err = p.ApplyPreset(shipped, role, nil); err != nil {
			t.Fatal(err)
		}
		for id, w := range want {
			if f := p.Features[id]; f != w {
				t.Errorf("after %s: %s = %+v, want %+v", role, id, f, w)
			}
		}
	}
}

func TestApplyPresetChartSwitchAnswers(t *testing.T) {
	b := answerBundles(t)
	open := []OpenQuestion{{Field: TargetFieldPrefix + FieldChart}, {Field: TargetFieldPrefix + FieldChartVersion}, {Field: answerFoundations}}
	p := preset(t, b, model.RoleConsumer, nil, nil)
	p.Open = slices.Clone(open)
	p.Answers = map[string]any{answerTarget: "dsc-a", answerFoundations: "Acme", answerConfig: true, answerUnknown: 1}

	tests := []struct {
		role, wantChart string
		wantCarried     []string
		wantDropped     []string
	}{
		// Only answerFoundations is declared by the trust-anchor bundle; answerTarget is placed by a rule only.
		{model.RoleOperator, taChart, []string{answerFoundations}, []string{answerConfig, answerUnknown, answerTarget}},
		// The way back starts from the operator's result, which holds answerFoundations only.
		{model.RoleConsumer, dscChart, []string{answerFoundations}, []string{}},
	}
	for _, tc := range tests {
		before := p
		got, sw, err := p.ApplyPreset(b, tc.role, nil)
		if err != nil {
			t.Fatal(err)
		}
		if sw == nil || !slices.Equal(sw.CarriedAnswers, tc.wantCarried) || !slices.Equal(sw.DroppedAnswers, tc.wantDropped) {
			t.Fatalf("%s: switch = %+v, want carried %v, dropped %v", tc.role, sw, tc.wantCarried, tc.wantDropped)
		}
		wantAnswers := map[string]any{}
		for _, id := range tc.wantCarried {
			wantAnswers[id] = before.Answers[id]
		}
		if !maps.Equal(got.Answers, wantAnswers) {
			t.Errorf("%s: answers = %v, want %v", tc.role, got.Answers, wantAnswers)
		}
		wantTarget := before.Target
		wantTarget.Chart, wantTarget.ChartVersion = tc.wantChart, ""
		if got.Target != wantTarget {
			t.Errorf("%s: target = %+v, want %+v", tc.role, got.Target, wantTarget)
		}
		if !slices.Equal(got.Open, open) {
			t.Errorf("%s: open = %v, want %v unchanged", tc.role, got.Open, open)
		}
		p = got
	}
}
