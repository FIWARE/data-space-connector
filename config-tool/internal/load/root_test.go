package load

import (
	"errors"
	"slices"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// wrapped is the chart's values packaged under `dsc:` by an Argo CD or umbrella chart, next to
// charts of its own whose names the connector also uses.
const wrapped = "grafana:\n  enabled: true\n" +
	"tempo:\n  enabled: false\n" +
	"dsc:\n" +
	"  tracing:\n    enabled: true\n" +
	"  tempo:\n    enabled: true\n" +
	"  did:\n    enabled: true\n" +
	"  rainbow:\n    ingress:\n      host: rainbow.example.org\n"

func rootPath(t *testing.T, s string) *roundtrip.Path {
	t.Helper()
	p, err := roundtrip.ParsePath(s)
	if err != nil {
		t.Fatal(err)
	}
	return &p
}

func TestLoadRoot(t *testing.T) {
	bundles := fixtureBundleList(t)
	tests := []struct {
		name           string
		src            string
		root           string
		requested      bool
		wantRoot       string
		wantFrom       RootSource
		wantCandidates []string
		leaves         map[string]string
		answers        map[string]answerWant
	}{
		{
			name:           "plain file is read from the document root",
			src:            "did:\n  enabled: true\n",
			wantRoot:       "",
			wantFrom:       RootDetected,
			wantCandidates: []string{""},
			leaves:         map[string]string{"did.enabled": "recognized"},
			answers:        map[string]answerWant{"identity.did.enabled": {true, FromRecognized, []string{"did.enabled"}}},
		},
		{
			name:           "values wrapped under a key next to same-named charts",
			src:            wrapped,
			wantRoot:       "dsc",
			wantFrom:       RootDetected,
			wantCandidates: []string{"dsc", ""},
			leaves: map[string]string{
				"grafana.enabled":          "unknown",
				"tempo.enabled":            "unknown",
				"dsc.did.enabled":          "recognized",
				"dsc.rainbow.ingress.host": "recognized",
				"dsc.tracing.enabled":      "unknown",
			},
			answers: map[string]answerWant{
				"identity.did.enabled": {true, FromRecognized, []string{"dsc.did.enabled"}},
				"domain.base":          {"example.org", FromRecognized, []string{"dsc.rainbow.ingress.host"}},
			},
		},
		{
			name:           "a rule path outside the root does not answer",
			src:            "did:\n  enabled: false\nconnector:\n  did:\n    enabled: true\n  scorpio:\n    enabled: true\n",
			wantRoot:       "connector",
			wantFrom:       RootDetected,
			wantCandidates: []string{"connector", ""},
			leaves:         map[string]string{"did.enabled": "unknown", "connector.did.enabled": "recognized"},
		},
		{
			name:           "a stray section of an older chart does not outrank the document root",
			src:            "did:\n  enabled: true\nscorpio:\n  enabled: true\nrainbow:\n  ingress:\n    host: rainbow.example.org\nlegacy:\n  tracing:\n    enabled: true\n",
			wantRoot:       "",
			wantFrom:       RootDetected,
			wantCandidates: []string{"", "legacy"},
			leaves: map[string]string{
				"did.enabled":            "recognized",
				"scorpio.enabled":        "recognized",
				"rainbow.ingress.host":   "recognized",
				"legacy.tracing.enabled": "unknown",
			},
			answers: map[string]answerWant{
				"identity.did.enabled": {true, FromRecognized, []string{"did.enabled"}},
				"data.scorpio.enabled": {true, FromRecognized, []string{"scorpio.enabled"}},
				"domain.base":          {"example.org", FromRecognized, []string{"rainbow.ingress.host"}},
			},
		},
		{
			name:           "global is shared by every chart",
			src:            "global:\n  domain: example.org\ndsc:\n  did:\n    enabled: true\n",
			wantRoot:       "dsc",
			wantFrom:       RootDetected,
			wantCandidates: []string{"dsc"},
			leaves:         map[string]string{"global.domain": "unknown", "dsc.did.enabled": "recognized"},
		},
		{
			name:           "nested wrapper within the depth limit",
			src:            "apps:\n  connector:\n    values:\n      did:\n        enabled: true\n",
			wantRoot:       "apps.connector.values",
			wantFrom:       RootDetected,
			wantCandidates: []string{"apps.connector.values"},
			leaves:         map[string]string{"apps.connector.values.did.enabled": "recognized"},
		},
		{
			name:           "wrapper beyond the depth limit is not found",
			src:            "a:\n  b:\n    c:\n      d:\n        did:\n          enabled: true\n",
			wantRoot:       "",
			wantFrom:       RootDetected,
			wantCandidates: []string{},
			leaves:         map[string]string{"a.b.c.d.did.enabled": "unknown"},
		},
		{
			name:           "no root is searched below a chart key",
			src:            "keycloak:\n  extra:\n    did:\n      enabled: true\n",
			wantRoot:       "",
			wantFrom:       RootDetected,
			wantCandidates: []string{""},
			leaves:         map[string]string{"keycloak.extra.did.enabled": "unknown"},
		},
		{
			name:           "requested document root overrides detection",
			src:            wrapped,
			root:           "",
			requested:      true,
			wantRoot:       "",
			wantFrom:       RootRequested,
			wantCandidates: []string{"dsc", ""},
			leaves:         map[string]string{"dsc.did.enabled": "unknown", "grafana.enabled": "unknown"},
		},
		{
			name:           "requested wrapper key",
			src:            wrapped,
			root:           "dsc",
			requested:      true,
			wantRoot:       "dsc",
			wantFrom:       RootRequested,
			wantCandidates: []string{"dsc", ""},
			leaves:         map[string]string{"dsc.did.enabled": "recognized"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts Options
			if tt.requested {
				opts.Root = rootPath(t, tt.root)
			}
			res, err := Load([]byte(tt.src), "values.yaml", bundles, opts)
			if err != nil {
				t.Fatal(err)
			}
			assertInvariants(t, []byte(tt.src), res)
			r := res.Report
			if r.Root != tt.wantRoot || res.Root.String() != tt.wantRoot || r.RootFrom != tt.wantFrom {
				t.Errorf("root = %q (%q) from %s, want %q from %s", r.Root, res.Root, r.RootFrom, tt.wantRoot, tt.wantFrom)
			}
			if !slices.Equal(r.RootCandidates, tt.wantCandidates) {
				t.Errorf("candidates = %q, want %q", r.RootCandidates, tt.wantCandidates)
			}
			entries := byPath(r)
			for path, class := range tt.leaves {
				if got := entries[path].class; got != class {
					t.Errorf("%s: class %q, want %q", path, got, class)
				}
			}
			if tt.answers != nil {
				assertAnswers(t, r.Answers, tt.answers)
			}
		})
	}
}

func TestLoadRootErrors(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{"missing key", "nowhere"},
		{"scalar", "dsc.did.enabled"},
		{"sequence index", "list[0]"},
	}
	src := []byte(wrapped + "list:\n  - did:\n      enabled: true\n")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(src, "values.yaml", fixtureBundleList(t), Options{Root: rootPath(t, tt.root)})
			if !errors.Is(err, ErrRoot) {
				t.Errorf("err = %v, want ErrRoot", err)
			}
		})
	}
}
