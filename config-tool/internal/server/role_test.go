package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	scorpioOff     = "scorpio:\n  enabled: false\n"
	scorpioOn      = "scorpio:\n  enabled: true\n"
	featureScorpio = "data.scorpio"
)

func rolesPath(chart, version string) string {
	q := url.Values{}
	if chart != "" {
		q.Set(queryChart, chart)
	}
	if version != "" {
		q.Set(queryVersion, version)
	}
	return "/api/roles?" + q.Encode()
}

func TestRolesEndpoint(t *testing.T) {
	bundles := mustBundles(t, newProfileSession(t).srv)
	taVersion := latestBundle(t, bundles, trustAnchorChart).Version
	catalogueFeatures := len(latestBundle(t, bundles, dscChart).Features)
	tests := []struct {
		name    string
		chart   string
		version string
		unauth  bool
		want    int
	}{
		{name: "latest DSC", chart: dscChart, want: http.StatusOK},
		{name: "DSC version", chart: dscChart, version: latestChart, want: http.StatusOK},
		{name: "trust-anchor falls back to the DSC catalogue", chart: trustAnchorChart, version: taVersion, want: http.StatusOK},
		{name: "unknown chart", chart: "other", want: http.StatusNotFound},
		{name: "unknown version", chart: dscChart, version: "0.0.1", want: http.StatusNotFound},
		{name: "no chart", want: http.StatusNotFound},
		{name: "missing token", chart: dscChart, unauth: true, want: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			req := request{path: rolesPath(tc.chart, tc.version), cookie: ps.cookie}
			if tc.unauth {
				req.cookie = nil
			}
			resp, raw := do(t, ps.srv, req)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, raw)
			}
			if resp.StatusCode != http.StatusOK {
				return
			}
			var got rolesResponse
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("body %q is not a role catalogue: %v", raw, err)
			}
			ids := make([]string, 0, len(got.Roles))
			for _, r := range got.Roles {
				ids = append(ids, r.ID)
				if (r.Floor == nil) != (r.ID == model.RoleCustom) {
					t.Errorf("role %s floor = %+v, want one exactly for the non-custom roles", r.ID, r.Floor)
				}
				if len(r.Features) != catalogueFeatures {
					t.Errorf("role %s offers %d features, want %d", r.ID, len(r.Features), catalogueFeatures)
				}
			}
			if !slices.Equal(ids, model.KnownRoles) {
				t.Errorf("roles = %v, want %v", ids, model.KnownRoles)
			}
			if len(got.Features) != catalogueFeatures || got.Features[0].Title == "" || got.Features[0].Group == "" {
				t.Errorf("features = %+v, want %d with title and group", got.Features, catalogueFeatures)
			}
			for i, f := range latestBundle(t, bundles, dscChart).Features {
				if i < len(got.Features) && got.Features[i].Consequence != f.Consequence {
					t.Errorf("feature %s consequence = %q, want %q", f.ID, got.Features[i].Consequence, f.Consequence)
				}
			}
		})
	}
}

func TestPresetEndpoint(t *testing.T) {
	fresh := profile.Profile{Target: profile.Target{Chart: dscChart}, Model: profile.ModelRef{Chart: dscChart}}
	operator := profile.Profile{Target: profile.Target{Chart: trustAnchorChart}, Role: model.RoleOperator,
		Features: map[string]profile.FeatureState{"ta.registry": {Enabled: true, Source: profile.SourcePreset}}}
	bundles := mustBundles(t, newProfileSession(t).srv)
	dscLatest := modelRef(*latestBundle(t, bundles, dscChart))
	taLatest := modelRef(*latestBundle(t, bundles, trustAnchorChart))
	tests := []struct {
		name    string
		load    string
		sha     string
		profile profile.Profile
		role    string
		unauth  bool
		want    int
		// wantErr is a substring of the refusal.
		wantErr       string
		wantModel     profile.ModelRef
		wantCatalogue *profile.ModelRef
		wantSwitch    *profile.ChartSwitch
		wantScorpio   *profile.FeatureState
	}{
		{name: "fresh configuration picks provider", profile: fresh, role: model.RoleProvider, want: http.StatusOK,
			wantModel: dscLatest, wantScorpio: &profile.FeatureState{Enabled: true, Source: profile.SourcePreset}},
		{name: "fresh configuration picks operator", profile: fresh, role: model.RoleOperator, want: http.StatusOK,
			wantModel: taLatest, wantCatalogue: &dscLatest,
			wantSwitch: &profile.ChartSwitch{FromChart: dscChart, ToChart: trustAnchorChart, DroppedChoices: []string{}, CarriedAnswers: []string{}, DroppedAnswers: []string{}}},
		{name: "operator drops the chosen version", profile: profile.Profile{Target: completeTarget()}, role: model.RoleOperator,
			want: http.StatusOK, wantModel: taLatest, wantCatalogue: &dscLatest,
			wantSwitch: &profile.ChartSwitch{FromChart: dscChart, FromVersion: latestChart, ToChart: trustAnchorChart, DroppedChoices: []string{}, CarriedAnswers: []string{}, DroppedAnswers: []string{}}},
		{name: "operator back to provider", profile: operator, role: model.RoleProvider, want: http.StatusOK,
			wantModel: dscLatest, wantSwitch: &profile.ChartSwitch{FromChart: trustAnchorChart, ToChart: dscChart, DroppedChoices: []string{}, CarriedAnswers: []string{}, DroppedAnswers: []string{}},
			wantScorpio: &profile.FeatureState{Enabled: true, Source: profile.SourcePreset}},
		{name: "consumer does not offer scorpio", profile: profile.Profile{Target: completeTarget()}, role: model.RoleConsumer,
			want: http.StatusOK, wantModel: dscLatest},
		{name: "unknown role", profile: fresh, role: "admin", want: http.StatusUnprocessableEntity, wantErr: "unknown role"},
		{name: "invalid profile", profile: profile.Profile{Target: completeTarget(), Role: model.RoleOperator},
			role: model.RoleProvider, want: http.StatusUnprocessableEntity, wantErr: "invalid profile"},
		{name: "stale sourceSha256", profile: profile.Profile{Target: completeTarget()}, sha: "0000", role: model.RoleProvider,
			want: http.StatusUnprocessableEntity, wantErr: errSourceMismatch.Error()},
		{name: "loaded toggle wins over the preset", load: scorpioOff, profile: profile.Profile{Target: completeTarget()},
			role: model.RoleProvider, want: http.StatusOK, wantModel: dscLatest,
			wantScorpio: &profile.FeatureState{Enabled: false, Source: profile.SourceLoaded}},
		{name: "loaded toggle the role does not offer is kept", load: scorpioOn, profile: profile.Profile{Target: completeTarget()},
			role: model.RoleConsumer, want: http.StatusOK, wantModel: dscLatest,
			wantScorpio: &profile.FeatureState{Enabled: true, Source: profile.SourceLoaded}},
		{name: "operator with a loaded source", load: scorpioOff, profile: profile.Profile{Target: completeTarget()},
			role: model.RoleOperator, want: http.StatusUnprocessableEntity, wantErr: profile.ErrChartSwitchLoaded.Error()},
		{name: "missing token", profile: fresh, role: model.RoleProvider, unauth: true, want: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			sha := tc.sha
			if tc.load != "" {
				sha = ps.load(valuesName, tc.load).SHA256
			}
			body, err := json.Marshal(presetRequest{Profile: tc.profile, Role: tc.role, SourceSHA256: sha})
			if err != nil {
				t.Fatal(err)
			}
			req := request{method: http.MethodPost, path: "/api/preset", origin: ownOrigin(ps.srv), cookie: ps.cookie, body: string(body)}
			if tc.unauth {
				req.cookie = nil
			}
			resp, raw := do(t, ps.srv, req)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, raw)
			}
			if resp.StatusCode != http.StatusOK {
				if !strings.Contains(raw, tc.wantErr) {
					t.Errorf("body %s does not contain %q", raw, tc.wantErr)
				}
				return
			}
			var got presetResponse
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("body %q is not a preset response: %v", raw, err)
			}
			p := got.Profile
			if p.Role != tc.role || p.Model != tc.wantModel || p.Target.Chart != tc.wantModel.Chart {
				t.Errorf("role, model, chart = %s, %+v, %s, want %s, %+v", p.Role, p.Model, p.Target.Chart, tc.role, tc.wantModel)
			}
			if c := p.Catalogue; (c == nil) != (tc.wantCatalogue == nil) || c != nil && *c != *tc.wantCatalogue {
				t.Errorf("catalogue = %+v, want %+v", c, tc.wantCatalogue)
			}
			if sw := got.ChartSwitch; (sw == nil) != (tc.wantSwitch == nil) || sw != nil && !reflect.DeepEqual(*sw, *tc.wantSwitch) {
				t.Errorf("chartSwitch = %+v, want %+v", sw, tc.wantSwitch)
			}
			if tc.wantSwitch != nil && p.Target.ChartVersion != "" {
				t.Errorf("chartVersion = %q after a switch, want it cleared", p.Target.ChartVersion)
			}
			if f, ok := p.Features[featureScorpio]; ok != (tc.wantScorpio != nil) || ok && f != *tc.wantScorpio {
				t.Errorf("%s = %+v (present %t), want %+v", featureScorpio, f, ok, tc.wantScorpio)
			}
			if (p.Source != nil) != (tc.load != "") {
				t.Errorf("source = %+v, want one exactly when a file is loaded", p.Source)
			}
			if errs := p.Validate(mustBundles(t, ps.srv)); len(errs) > 0 {
				t.Errorf("preset profile does not validate: %v", errs)
			}
		})
	}
}

const (
	featureVerification = "access.verification"
	featureMarket       = "market.local"
	featureRegistry     = "ta.registry"
)

func TestFeaturesEndpoint(t *testing.T) {
	provider := profile.Profile{Target: completeTarget(), Role: model.RoleProvider}
	consumer := profile.Profile{Target: completeTarget(), Role: model.RoleConsumer}
	tests := []struct {
		name    string
		profile profile.Profile
		toggle  *featureToggle
		unauth  bool
		want    int
		// wantErr is a substring of the refusal.
		wantErr string
		// wantChoice is the choice of toggle.Feature after the call.
		wantChoice *profile.FeatureChoice
	}{
		{name: "choices without a toggle", profile: provider, want: http.StatusOK},
		{name: "recommended feature turned off", profile: provider, toggle: &featureToggle{Feature: featureVerification},
			want: http.StatusOK, wantChoice: &profile.FeatureChoice{ID: featureVerification, Offer: model.OfferRecommended,
				Preset: true, Enabled: false, Deliberate: true, Source: profile.SourceUser}},
		{name: "not-offered feature of the chart turned on", profile: consumer, toggle: &featureToggle{Feature: featureMarket, Enabled: true},
			want: http.StatusOK, wantChoice: &profile.FeatureChoice{ID: featureMarket, Offer: model.OfferNotOffered,
				Preset: false, Enabled: true, Deliberate: true, Source: profile.SourceUser}},
		{name: "no role", profile: profile.Profile{Target: completeTarget()}, want: http.StatusUnprocessableEntity,
			wantErr: profile.ErrNoRole.Error()},
		{name: "unknown feature", profile: provider, toggle: &featureToggle{Feature: "no.such", Enabled: true},
			want: http.StatusUnprocessableEntity, wantErr: profile.ErrUnknownFeature.Error()},
		{name: "feature of the other chart", profile: provider, toggle: &featureToggle{Feature: featureRegistry, Enabled: true},
			want: http.StatusUnprocessableEntity, wantErr: profile.ErrFeatureOfOtherChart.Error()},
		{name: "missing token", profile: provider, unauth: true, want: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			status, raw, got := ps.features(featuresRequest{Profile: tc.profile, Toggle: tc.toggle}, tc.unauth)
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, raw)
			}
			if status != http.StatusOK {
				if !strings.Contains(raw, tc.wantErr) {
					t.Errorf("body %s does not contain %q", raw, tc.wantErr)
				}
				return
			}
			bundles := mustBundles(t, ps.srv)
			if want, err := got.Profile.FeatureChoices(bundles); err != nil || !reflect.DeepEqual(got.Choices, want) {
				t.Errorf("choices = %+v, want the profile's %+v (err %v)", got.Choices, want, err)
			}
			if slices.ContainsFunc(got.Choices, func(c profile.FeatureChoice) bool { return c.ID == featureRegistry }) {
				t.Errorf("choices list %s of the other chart", featureRegistry)
			}
			if tc.wantChoice != nil {
				if c := choiceOf(got.Choices, tc.wantChoice.ID); c != *tc.wantChoice {
					t.Errorf("choice = %+v, want %+v", c, *tc.wantChoice)
				}
			}
			if errs := got.Profile.Validate(bundles); len(errs) > 0 {
				t.Errorf("profile does not validate: %v", errs)
			}
		})
	}
}

// TestDeliberateChoiceSurvivesSaveAndOpen is the acceptance round trip of US-2.2.
func TestDeliberateChoiceSurvivesSaveAndOpen(t *testing.T) {
	ps := newProfileSession(t)
	p := ps.mustPreset(profile.Profile{Target: completeTarget()}, model.RoleProvider).Profile
	p = ps.mustToggle(p, featureVerification, false).Profile
	path := filepath.Join(t.TempDir(), profileName)
	ps.mustSave(saveRequest{Path: path, Profile: p})
	status, raw, opened := ps.open(path)
	if status != http.StatusOK {
		t.Fatalf("open status = %d: %s", status, raw)
	}
	want := profile.FeatureState{Enabled: false, Source: profile.SourceUser}
	if f := opened.Profile.Features[featureVerification]; f != want {
		t.Errorf("opened %s = %+v, want %+v", featureVerification, f, want)
	}
	status, raw, got := ps.features(featuresRequest{Profile: opened.Profile}, false)
	if status != http.StatusOK {
		t.Fatalf("features status = %d: %s", status, raw)
	}
	if c := choiceOf(got.Choices, featureVerification); !c.Deliberate || c.Enabled || c.Source != profile.SourceUser {
		t.Errorf("choice after open = %+v, want a deliberate user choice that is off", c)
	}
}

func TestRoleSwitchKeepsUserChoices(t *testing.T) {
	tests := []struct {
		name string
		role string
		// wantState is the toggled feature's state after the switch; nil when the switch drops it.
		wantState   *profile.FeatureState
		wantDropped []string
	}{
		{name: "same chart keeps the choice", role: model.RoleConsumerProvider,
			wantState: &profile.FeatureState{Enabled: false, Source: profile.SourceUser}},
		{name: "chart switch names the dropped choice", role: model.RoleOperator, wantDropped: []string{featureVerification}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			p := ps.mustPreset(profile.Profile{Target: completeTarget()}, model.RoleProvider).Profile
			p = ps.mustToggle(p, featureVerification, false).Profile
			got := ps.mustPreset(p, tc.role)
			if f, ok := got.Profile.Features[featureVerification]; ok != (tc.wantState != nil) || ok && f != *tc.wantState {
				t.Errorf("%s = %+v (present %t), want %+v", featureVerification, f, ok, tc.wantState)
			}
			var dropped []string
			if got.ChartSwitch != nil {
				dropped = got.ChartSwitch.DroppedChoices
			}
			if !slices.Equal(dropped, tc.wantDropped) {
				t.Errorf("droppedChoices = %v, want %v", dropped, tc.wantDropped)
			}
		})
	}
}

// features posts req to /api/features, without the session cookie when unauth is set.
func (ps *profileSession) features(req featuresRequest, unauth bool) (int, string, featuresResponse) {
	ps.t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		ps.t.Fatal(err)
	}
	r := request{method: http.MethodPost, path: "/api/features", origin: ownOrigin(ps.srv), cookie: ps.cookie, body: string(data)}
	if unauth {
		r.cookie = nil
	}
	resp, raw := do(ps.t, ps.srv, r)
	var got featuresResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			ps.t.Fatalf("body %q is not a features response: %v", raw, err)
		}
	}
	return resp.StatusCode, raw, got
}

// mustToggle sets feature id through the API and fails the test on any status but 200.
func (ps *profileSession) mustToggle(p profile.Profile, id string, enabled bool) featuresResponse {
	ps.t.Helper()
	status, raw, got := ps.features(featuresRequest{Profile: p, Toggle: &featureToggle{Feature: id, Enabled: enabled}}, false)
	if status != http.StatusOK {
		ps.t.Fatalf("features status = %d: %s", status, raw)
	}
	return got
}

// mustPreset applies role through the API and fails the test on any status but 200.
func (ps *profileSession) mustPreset(p profile.Profile, role string) presetResponse {
	ps.t.Helper()
	data, err := json.Marshal(presetRequest{Profile: p, Role: role})
	if err != nil {
		ps.t.Fatal(err)
	}
	resp, raw := do(ps.t, ps.srv, request{method: http.MethodPost, path: "/api/preset", origin: ownOrigin(ps.srv), cookie: ps.cookie, body: string(data)})
	if resp.StatusCode != http.StatusOK {
		ps.t.Fatalf("preset status = %d: %s", resp.StatusCode, raw)
	}
	var got presetResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		ps.t.Fatalf("body %q is not a preset response: %v", raw, err)
	}
	return got
}

func choiceOf(choices []profile.FeatureChoice, id string) profile.FeatureChoice {
	i := slices.IndexFunc(choices, func(c profile.FeatureChoice) bool { return c.ID == id })
	if i < 0 {
		return profile.FeatureChoice{}
	}
	return choices[i]
}

func mustBundles(t *testing.T, srv *Server) []model.Bundle {
	t.Helper()
	bundles, err := model.Versions(srv.models)
	if err != nil {
		t.Fatal(err)
	}
	return bundles
}

// latestBundle returns the shipped Latest bundle of chart, so tests survive revision bumps and catalogue growth.
func latestBundle(t *testing.T, bundles []model.Bundle, chart string) *model.Bundle {
	t.Helper()
	i := slices.IndexFunc(bundles, func(b model.Bundle) bool { return b.Chart == chart && b.Latest })
	if i < 0 {
		t.Fatalf("no latest bundle for chart %s", chart)
	}
	return &bundles[i]
}
