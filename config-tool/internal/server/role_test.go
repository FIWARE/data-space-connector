package server

import (
	"encoding/json"
	"net/http"
	"net/url"
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
			wantSwitch: &profile.ChartSwitch{FromChart: dscChart, ToChart: trustAnchorChart}},
		{name: "operator drops the chosen version", profile: profile.Profile{Target: completeTarget()}, role: model.RoleOperator,
			want: http.StatusOK, wantModel: taLatest, wantCatalogue: &dscLatest,
			wantSwitch: &profile.ChartSwitch{FromChart: dscChart, FromVersion: latestChart, ToChart: trustAnchorChart}},
		{name: "operator back to provider", profile: operator, role: model.RoleProvider, want: http.StatusOK,
			wantModel: dscLatest, wantSwitch: &profile.ChartSwitch{FromChart: trustAnchorChart, ToChart: dscChart},
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
		{name: "loaded toggle the role does not offer", load: scorpioOn, profile: profile.Profile{Target: completeTarget()},
			role: model.RoleConsumer, want: http.StatusUnprocessableEntity, wantErr: featureScorpio},
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
			if sw := got.ChartSwitch; (sw == nil) != (tc.wantSwitch == nil) || sw != nil && *sw != *tc.wantSwitch {
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
