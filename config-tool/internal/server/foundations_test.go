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
	foundationOrg    = "org.displayName"
)

// foundationJSON is a decoded foundation, target paths kept as strings.
type foundationJSON struct {
	ID         string   `json:"id"`
	Stage      string   `json:"stage"`
	Default    string   `json:"default"`
	Label      string   `json:"label"`
	Count      int      `json:"count"`
	Components []string `json:"components"`
	Targets    []struct {
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
				if f.Count != len(f.Targets) || f.Label == "" || f.Stage == "" {
					t.Errorf("foundation %s = %+v, want a label, a stage and count = len(targets)", f.ID, f)
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
	if org.Count != len(org.Targets) {
		t.Errorf("%s count = %d, want %d", foundationOrg, org.Count, len(org.Targets))
	}
}
