package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"testing"
)

const (
	foundationDomain = "domain.base"
	foundationHost   = "host.keycloak"
	// hostFeature is the only feature that gates foundationHost's display.
	hostFeature   = "identity.issuer"
	foundationOrg = "org.displayName"
	foundationDID = "did.value"
)

// foundationJSON is a decoded foundation, target paths kept as strings.
type foundationJSON struct {
	ID      string `json:"id"`
	Stage   string `json:"stage"`
	Group   string `json:"group"`
	Default string `json:"default"`
	// DefaultWhen is nil when the response holds null.
	DefaultWhen *struct {
		ID     string `json:"id"`
		Equals string `json:"equals"`
	} `json:"defaultWhen"`
	Prefix string `json:"prefix"`
	// Options is nil only when the response holds null.
	Options    []json.RawMessage `json:"options"`
	Label      string            `json:"label"`
	Count      int               `json:"count"`
	Components []string          `json:"components"`
	// Features is nil only when the response holds null.
	Features []string `json:"features"`
	Targets  []struct {
		Path      string `json:"path"`
		Component string `json:"component"`
	} `json:"targets"`
}

// getFoundations fetches path and decodes a 200 response.
func getFoundations(t *testing.T, ps *profileSession, path string, cookie bool) (int, []foundationJSON) {
	t.Helper()
	req := request{path: path}
	if cookie {
		req.cookie = ps.cookie
	}
	resp, raw := do(t, ps.srv, req)
	var got struct {
		Foundations []foundationJSON `json:"foundations"`
	}
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("body is not a foundations response: %v", err)
		}
	}
	return resp.StatusCode, got.Foundations
}

func foundationsPath(chart, version string) string {
	q := url.Values{}
	if chart != "" {
		q.Set(queryChart, chart)
	}
	if version != "" {
		q.Set(queryVersion, version)
	}
	return "/api/foundations?" + q.Encode()
}

func TestFoundationsEndpoint(t *testing.T) {
	ps := newProfileSession(t)
	taVersion := latestBundle(t, mustBundles(t, ps.srv), trustAnchorChart).Version
	tests := []struct {
		name    string
		chart   string
		version string
		unauth  bool
		want    int
		// wantIDs must be listed, in this order, among the foundations.
		wantIDs []string
	}{
		{name: "latest DSC", chart: dscChart, want: http.StatusOK, wantIDs: []string{foundationOrg, foundationDomain, foundationHost}},
		{name: "DSC version", chart: dscChart, version: latestChart, want: http.StatusOK, wantIDs: []string{foundationOrg, foundationDomain}},
		{name: "trust anchor declares without targets", chart: trustAnchorChart, version: taVersion, want: http.StatusOK, wantIDs: []string{foundationOrg, foundationDomain}},
		{name: "unknown version", chart: dscChart, version: "0.0.1", want: http.StatusNotFound},
		{name: "no chart", want: http.StatusNotFound},
		{name: "missing token", chart: dscChart, unauth: true, want: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, foundations := getFoundations(t, ps, foundationsPath(tc.chart, tc.version), !tc.unauth)
			if status != tc.want {
				t.Fatalf("status = %d, want %d", status, tc.want)
			}
			var ids []string
			for _, f := range foundations {
				ids = append(ids, f.ID)
				if f.Count != len(f.Targets) || f.Label == "" || f.Stage == "" || f.Options == nil || f.Features == nil {
					t.Errorf("foundation %s = %+v, want a label, a stage, options and features lists and count = len(targets)", f.ID, f)
				}
			}
			var listed []string
			for _, id := range ids {
				if slices.Contains(tc.wantIDs, id) {
					listed = append(listed, id)
				}
			}
			if !slices.Equal(listed, tc.wantIDs) {
				t.Errorf("foundations = %v, want %v in that order", ids, tc.wantIDs)
			}
		})
	}
}

func TestFoundationsFanOut(t *testing.T) {
	ps := newProfileSession(t)
	status, foundations := getFoundations(t, ps, foundationsPath(dscChart, latestChart), true)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	byID := map[string]foundationJSON{}
	for _, f := range foundations {
		byID[f.ID] = f
	}
	domain, host, org := byID[foundationDomain], byID[foundationHost], byID[foundationOrg]
	if host.Default == "" || domain.Default != "" {
		t.Errorf("defaults: %s = %q, %s = %q, want only the host derived", foundationHost, host.Default, foundationDomain, domain.Default)
	}
	if host.Count == 0 || domain.Count <= host.Count || len(domain.Components) <= len(host.Components) {
		t.Errorf("fan-out: %s reaches %d keys in %v, %s %d in %v; want the domain to reach more",
			foundationDomain, domain.Count, domain.Components, foundationHost, host.Count, host.Components)
	}
	if !slices.IsSorted(domain.Components) || !slices.Equal(host.Components, []string{"keycloak"}) {
		t.Errorf("components: domain %v, host %v, want sorted and keycloak only for the host", domain.Components, host.Components)
	}
	if !slices.Equal(host.Features, []string{hostFeature}) || len(domain.Features) != 0 {
		t.Errorf("features: %s %v, %s %v, want %s gated by %s only", foundationHost, host.Features, foundationDomain, domain.Features, foundationHost, hostFeature)
	}
	if org.Count != len(org.Targets) {
		t.Errorf("%s count = %d, want %d", foundationOrg, org.Count, len(org.Targets))
	}
}

func TestFoundationsGroupAndCondition(t *testing.T) {
	ps := newProfileSession(t)
	_, foundations := getFoundations(t, ps, foundationsPath(dscChart, latestChart), true)
	byID := map[string]foundationJSON{}
	for _, f := range foundations {
		byID[f.ID] = f
	}
	for id, want := range map[string]string{foundationOrg: "organization", foundationDID: "organization", foundationDomain: "network", foundationHost: "network"} {
		if got := byID[id].Group; got != want {
			t.Errorf("%s group = %q, want %q", id, got, want)
		}
	}
	did := byID[foundationDID]
	if did.DefaultWhen == nil || did.DefaultWhen.ID != "did.method" || did.DefaultWhen.Equals != "did:web" || did.Prefix == "" {
		t.Errorf("%s = defaultWhen %+v, prefix %q; want did.method = did:web and a prefix", foundationDID, did.DefaultWhen, did.Prefix)
	}
	if host := byID[foundationHost]; host.DefaultWhen != nil || host.Prefix != "" {
		t.Errorf("%s = defaultWhen %+v, prefix %q; want neither", foundationHost, host.DefaultWhen, host.Prefix)
	}
}

func TestFoundationsDataspace(t *testing.T) {
	ps := newProfileSession(t)
	_, foundations := getFoundations(t, ps, foundationsPath(dscChart, latestChart), true)
	byID := map[string]foundationJSON{}
	for _, f := range foundations {
		byID[f.ID] = f
	}
	for id, count := range map[string]int{"dataspace.tirAddress": 1, "dataspace.credentialTypes": 0, "dataspace.autoRegistration": 0} {
		f, ok := byID[id]
		if !ok || f.Group != "dataspace" || f.Count != count {
			t.Errorf("%s = present %v, group %q, count %d; want group dataspace, count %d", id, ok, f.Group, f.Count, count)
		}
	}
}

func TestFoundationsResourceProfile(t *testing.T) {
	const (
		id         = "resources.profile"
		targets    = 20
		components = 5
	)
	ps := newProfileSession(t)
	_, foundations := getFoundations(t, ps, foundationsPath(dscChart, latestChart), true)
	i := slices.IndexFunc(foundations, func(f foundationJSON) bool { return f.ID == id })
	if i < 0 {
		t.Fatalf("%s is not listed", id)
	}
	if f := foundations[i]; f.Group != "platform" || f.Count != targets || len(f.Targets) != targets || len(f.Components) != components {
		t.Errorf("%s = group %q, count %d, %d targets in %v; want platform, %d targets in %d components",
			id, f.Group, f.Count, len(f.Targets), f.Components, targets, components)
	}
}

func TestFoundationsResources(t *testing.T) {
	type quantities struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
	}
	type spec struct {
		Requests quantities `json:"requests"`
		Limits   quantities `json:"limits"`
	}
	type table struct {
		Answer     string `json:"answer"`
		MeasuredOn string `json:"measuredOn"`
		Doc        string `json:"doc"`
		Components []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Path  string `json:"path"`
		} `json:"components"`
		Profiles map[string]map[string]spec `json:"profiles"`
	}
	ps := newProfileSession(t)
	taVersion := latestBundle(t, mustBundles(t, ps.srv), trustAnchorChart).Version
	tests := []struct {
		name  string
		path  string
		check func(t *testing.T, got *table)
	}{
		{name: "DSC previews the components, not the unsized ones", path: foundationsPath(dscChart, latestChart), check: func(t *testing.T, got *table) {
			if got == nil {
				t.Fatal("resources = null, want the table")
			}
			var ids []string
			for _, c := range got.Components {
				if c.Label == "" || c.Path == "" {
					t.Errorf("component %+v, want a label and a path", c)
				}
				ids = append(ids, c.ID)
			}
			wantIDs := []string{"keycloak", "scorpio", "tmforum", "til", "contractManagement"}
			if got.Answer != "resources.profile" || got.MeasuredOn == "" || got.Doc == "" || !slices.Equal(ids, wantIDs) {
				t.Errorf("resources = %+v, want answer resources.profile, measuredOn, doc and components %v", got, wantIDs)
			}
			want := spec{Requests: quantities{CPU: "500m", Memory: "2Gi"}, Limits: quantities{CPU: "2", Memory: "2Gi"}}
			if k := got.Profiles["production"]["keycloak"]; k != want || len(got.Profiles) != 2 {
				t.Errorf("profiles = %+v, want development and production, production keycloak %+v", got.Profiles, want)
			}
		}},
		{name: "trust anchor sizes nothing", path: foundationsPath(trustAnchorChart, taVersion), check: func(t *testing.T, got *table) {
			if got != nil {
				t.Errorf("resources = %+v, want null", got)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, raw := do(t, ps.srv, request{path: tc.path, cookie: ps.cookie})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			var got struct {
				Resources *table `json:"resources"`
			}
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("body is not a foundations response: %v", err)
			}
			tc.check(t, got.Resources)
		})
	}
}
