package render

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	fixtureDir  = "testdata"
	domain      = "example.org"
	displayName = "ACME"
	// header is the generated header for the fixture bundle's chart version.
	header = roundtrip.HeaderPrefix + "v0.2.0 for chart 1.0.0\n" + roundtrip.HeaderOwnedLine + "\n\n"
	// nestKey is the key the nested variant of a loaded file puts the chart's values under.
	nestKey = "dsc"
	// longRunes exceeds the load report's value preview.
	longRunes = 100
)

func fixtureBundles(t *testing.T) []model.Bundle {
	t.Helper()
	bs, err := model.Versions(os.DirFS(fixtureDir))
	if err != nil {
		t.Fatal(err)
	}
	return bs
}

func fixtureBundle(t *testing.T) model.Bundle {
	t.Helper()
	return fixtureBundles(t)[0]
}

func renderFixture(t *testing.T, answers map[string]any, opts Options) Tree {
	t.Helper()
	tree, err := Render(fixtureBundle(t), profile.Profile{Answers: answers}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func values(tree Tree) map[string]any {
	m := map[string]any{}
	for _, l := range tree.Leaves {
		m[l.Path.String()] = l.Value
	}
	return m
}

func leaf(t *testing.T, tree Tree, path string) Leaf {
	t.Helper()
	for _, l := range tree.Leaves {
		if l.Path.String() == path {
			return l
		}
	}
	t.Fatalf("no leaf at %s", path)
	return Leaf{}
}

func TestRenderValues(t *testing.T) {
	keycloak := map[string]any{
		"keycloak.ingress.hostname":                                "keycloak." + domain,
		"keycloak.ingress.tls[0].hosts[0]":                         "keycloak." + domain,
		"keycloak.ingress.tls[0].secretName":                       "keycloak." + domain + "-tls",
		`keycloak.ingress.annotations."cert-manager.io/alt-names"`: "keycloak." + domain + ",rainbow." + domain,
	}
	withDomain := merge(keycloak, map[string]any{
		"marketplace.url":      "https://market." + domain,
		"rainbow.ingress.host": "rainbow." + domain,
	})
	tests := []struct {
		name    string
		answers map[string]any
		want    map[string]any
	}{
		{name: "unanswered writes nothing", want: map[string]any{}},
		{
			name:    "copy keeps the answer",
			answers: map[string]any{"org.displayName": displayName},
			want:    map[string]any{"marketplace.title": displayName},
		},
		{
			name:    "a domain derives every host but leaves an incomplete owned list out",
			answers: map[string]any{"domain.base": domain},
			want:    withDomain,
		},
		{
			name:    "the last host completes the owned list",
			answers: map[string]any{"domain.base": domain, "host.scorpioAlt": "ld." + domain},
			want: merge(withDomain, map[string]any{
				"scorpio.ingress.hosts[0].host":     "scorpio." + domain,
				"scorpio.ingress.hosts[0].paths[0]": "/",
				"scorpio.ingress.hosts[1].host":     "ld." + domain,
				"scorpio.ingress.hosts[1].paths[0]": "/",
			}),
		},
		{
			name:    "a derived host is absent without a domain",
			answers: map[string]any{"host.keycloak": "sso.other.org"},
			want: map[string]any{
				"keycloak.ingress.hostname":          "sso.other.org",
				"keycloak.ingress.tls[0].hosts[0]":   "sso.other.org",
				"keycloak.ingress.tls[0].secretName": "sso.other.org-tls",
			},
		},
		{
			name:    "an answered host wins over its default",
			answers: map[string]any{"domain.base": domain, "host.keycloak": "sso.other.org"},
			want: merge(withDomain, map[string]any{
				"keycloak.ingress.hostname":                                "sso.other.org",
				"keycloak.ingress.tls[0].hosts[0]":                         "sso.other.org",
				"keycloak.ingress.tls[0].secretName":                       "sso.other.org-tls",
				`keycloak.ingress.annotations."cert-manager.io/alt-names"`: "sso.other.org,rainbow." + domain,
			}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := values(renderFixture(t, tc.answers, Options{})); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("values = %v, want %v", got, tc.want)
			}
		})
	}
}

func merge(ms ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func TestRenderErrors(t *testing.T) {
	tests := []struct {
		name    string
		answers map[string]any
		want    string
	}{
		{"an undeclared answer", map[string]any{"domain.other": domain}, `answer "domain.other" is not declared`},
		{"a derived host its type refuses", map[string]any{"domain.base": "Example.org"}, "derived host.keycloak"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Render(fixtureBundle(t), profile.Profile{Answers: tc.answers}, Options{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestRenderProvenance(t *testing.T) {
	tree := renderFixture(t, map[string]any{"domain.base": domain, "host.scorpioAlt": "ld." + domain}, Options{})
	tests := []struct {
		path string
		want Provenance
	}{
		{"marketplace.url", Provenance{Foundation: "domain.base", Answers: []string{"domain.base"}, Fn: "tpl"}},
		{`keycloak.ingress.annotations."cert-manager.io/alt-names"`,
			Provenance{Foundation: "host.keycloak", Answers: []string{"host.keycloak", "host.rainbow"}, Fn: "join", Target: 3}},
		{"rainbow.ingress.host", Provenance{Foundation: "host.rainbow", Answers: []string{"host.rainbow"}, Fn: "copy"}},
		{"scorpio.ingress.hosts[1].paths[0]", Provenance{Foundation: "host.scorpioAlt", Answers: []string{}, Fn: "const", Target: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := leaf(t, tree, tc.path).Provenance; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("provenance = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRenderSortsSegmentWise(t *testing.T) {
	tree := renderFixture(t, map[string]any{"domain.base": domain, "host.scorpioAlt": "ld." + domain}, Options{})
	var got []string
	for _, l := range tree.Leaves {
		got = append(got, l.Path.String())
	}
	want := []string{
		`keycloak.ingress.annotations."cert-manager.io/alt-names"`,
		"keycloak.ingress.hostname",
		"keycloak.ingress.tls[0].hosts[0]",
		"keycloak.ingress.tls[0].secretName",
		"marketplace.url",
		"rainbow.ingress.host",
		"scorpio.ingress.hosts[0].host",
		"scorpio.ingress.hosts[0].paths[0]",
		"scorpio.ingress.hosts[1].host",
		"scorpio.ingress.hosts[1].paths[0]",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

func TestNestedBuildsSlices(t *testing.T) {
	tree := renderFixture(t, map[string]any{"domain.base": domain, "host.scorpioAlt": "ld." + domain}, Options{})
	nested := tree.Nested()
	hosts := nested["scorpio"].(map[string]any)["ingress"].(map[string]any)["hosts"]
	wantHosts := []any{
		map[string]any{"host": "scorpio." + domain, "paths": []any{"/"}},
		map[string]any{"host": "ld." + domain, "paths": []any{"/"}},
	}
	if !reflect.DeepEqual(hosts, wantHosts) {
		t.Errorf("scorpio hosts = %v, want %v", hosts, wantHosts)
	}
	tls := nested["keycloak"].(map[string]any)["ingress"].(map[string]any)["tls"]
	wantTLS := []any{map[string]any{"hosts": []any{"keycloak." + domain}, "secretName": "keycloak." + domain + "-tls"}}
	if !reflect.DeepEqual(tls, wantTLS) {
		t.Errorf("keycloak tls = %v, want %v", tls, wantTLS)
	}
}

// pinWant is the expected state of one leaf after pinning; an unlisted leaf is re-rendered.
type pinWant struct {
	value    any
	kept     bool
	rendered any
}

func TestRenderPinsHandEdits(t *testing.T) {
	long := strings.Repeat("x", longRunes)
	tests := []struct {
		name string
		head string
		body string
		want map[string]pinWant
	}{
		{
			name: "recognized leaves are re-rendered",
			body: "keycloak:\n  ingress:\n    hostname: keycloak.example.org\nrainbow:\n  ingress:\n    host: rainbow.example.org\n",
		},
		{
			name: "owned leaves are re-rendered",
			head: header,
			body: "# dsc-config:owned domain.base\nkeycloak:\n  ingress:\n    hostname: keycloak.example.org\n",
		},
		{
			name: "a leaf that differs from the derived answer is pinned",
			body: "keycloak:\n  ingress:\n    hostname: keycloak.example.org\nrainbow:\n  ingress:\n    host: rainbow.other.org\n",
			want: map[string]pinWant{"rainbow.ingress.host": {value: "rainbow.other.org", kept: true, rendered: "rainbow." + domain}},
		},
		{
			name: "an unknown leaf longer than the preview is kept whole",
			body: "marketplace:\n  title: " + long + "\n",
			want: map[string]pinWant{"marketplace.title": {value: long, kept: true, rendered: displayName}},
		},
		{
			name: "an int is kept as an int",
			body: "marketplace:\n  title: 42\n",
			want: map[string]pinWant{"marketplace.title": {value: 42, kept: true, rendered: displayName}},
		},
		{
			name: "a collection at a target path is pinned without a kept value",
			body: "marketplace:\n  title:\n    a: 1\n",
			want: map[string]pinWant{"marketplace.title": {rendered: displayName}},
		},
		{
			name: "a scalar above a target path pins it without a kept value",
			body: "marketplace: x\n",
			want: map[string]pinWant{
				"marketplace.title": {rendered: displayName},
				"marketplace.url":   {rendered: "https://market." + domain},
			},
		},
		{
			name: "a shadowed duplicate is pinned and keeps the last occurrence",
			body: "rainbow:\n  ingress:\n    host: rainbow.a.org\n    host: rainbow.b.org\n",
			want: map[string]pinWant{"rainbow.ingress.host": {value: "rainbow.b.org", kept: true, rendered: "rainbow." + domain}},
		},
	}
	answers := map[string]any{"org.displayName": displayName, "domain.base": domain}
	for _, tc := range tests {
		variants := []struct {
			name string
			src  string
			root roundtrip.Path
		}{
			{"top level", tc.head + tc.body, roundtrip.Path{}},
			{"nested", tc.head + nestKey + ":\n" + indent(tc.body), mustPath(t, nestKey)},
		}
		for _, v := range variants {
			t.Run(tc.name+"/"+v.name, func(t *testing.T) {
				r, err := load.Load([]byte(v.src), "values.yaml", fixtureBundles(t), load.Options{Root: &v.root})
				if err != nil {
					t.Fatal(err)
				}
				tree := renderFixture(t, answers, Options{Overrides: r})
				nested := values(Tree{Leaves: nestedLeaves(tree)})
				for _, l := range tree.Leaves {
					key := l.Path.String()
					want, pinned := tc.want[key]
					p := l.Provenance
					if p.Pinned != pinned {
						t.Errorf("%s pinned = %v, want %v", key, p.Pinned, pinned)
						continue
					}
					if !pinned {
						if p.Rendered != nil || p.Kept {
							t.Errorf("%s re-rendered leaf has rendered %v, kept %v", key, p.Rendered, p.Kept)
						}
						continue
					}
					if l.Value != want.value || p.Kept != want.kept || p.Rendered != want.rendered {
						t.Errorf("%s = (value %v, kept %v, rendered %v), want %+v", key, l.Value, p.Kept, p.Rendered, want)
					}
					if _, inNested := nested[key]; inNested != want.kept {
						t.Errorf("%s in Nested() = %v, want %v", key, inNested, want.kept)
					}
				}
			})
		}
	}
}

// nestedLeaves keeps the leaves Nested() writes.
func nestedLeaves(tree Tree) []Leaf {
	var ls []Leaf
	for _, l := range tree.Leaves {
		if !l.Provenance.Pinned || l.Provenance.Kept {
			ls = append(ls, l)
		}
	}
	return ls
}

func TestNestedOmitsUnkeptPin(t *testing.T) {
	r, err := load.Load([]byte("marketplace:\n  title:\n    a: 1\n"), "values.yaml", fixtureBundles(t), load.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tree := renderFixture(t, map[string]any{"org.displayName": displayName, "domain.base": domain}, Options{Overrides: r})
	got := tree.Nested()["marketplace"]
	want := map[string]any{"url": "https://market." + domain}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("marketplace = %v, want %v", got, want)
	}
}

func indent(s string) string {
	lines := strings.SplitAfter(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "")
}

func mustPath(t *testing.T, s string) roundtrip.Path {
	t.Helper()
	p, err := roundtrip.ParsePath(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderJSONIsStable(t *testing.T) {
	r, err := load.Load([]byte("rainbow:\n  ingress:\n    host: rainbow.other.org\n"), "values.yaml", fixtureBundles(t), load.Options{})
	if err != nil {
		t.Fatal(err)
	}
	answers := map[string]any{"org.displayName": displayName, "domain.base": domain, "host.scorpioAlt": "ld." + domain}
	marshal := func() []byte {
		b, err := json.Marshal(renderFixture(t, answers, Options{Overrides: r}))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if first, second := marshal(), marshal(); !bytes.Equal(first, second) {
		t.Errorf("two renders differ:\n%s\n%s", first, second)
	}
}
