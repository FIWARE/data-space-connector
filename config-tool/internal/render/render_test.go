package render

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

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
	// scorpioAlt answers the host without a default, so the fixture's scorpio hosts list is complete.
	scorpioAlt = "ld." + domain
	// didFixture is the model package's choice-and-DID bundle.
	didFixture = "../model/testdata/foundations/did.yaml"
	didWeb     = "did:web:" + domain
	didKey     = "did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK"
	didPath    = "keycloak.issuerDid"
	// prefixFixture is the model package's bundle whose prefix names an answer its default does not.
	prefixFixture = "../model/testdata/foundations/prefix.yaml"
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

func modelFixture(t *testing.T, file string) model.Bundle {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := model.Versions(fstest.MapFS{"1.0.0/bundle.yaml": {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	return bs[0]
}

func renderDID(t *testing.T, answers map[string]any) (Tree, error) {
	t.Helper()
	return Render(modelFixture(t, didFixture), profile.Profile{Answers: answers}, Options{})
}

func TestRenderDefaultWhenAndPrefix(t *testing.T) {
	scorpio := map[string]any{"scorpio.ingress.host": "scorpio." + domain}
	tests := []struct {
		name    string
		answers map[string]any
		want    map[string]any
		wantErr string
	}{
		{name: "the constant method lets the DID derive", answers: map[string]any{"domain.base": domain},
			want: merge(scorpio, map[string]any{didPath: didWeb})},
		{name: "a given did:web derives too", answers: map[string]any{"domain.base": domain, "did.method": "did:web"},
			want: merge(scorpio, map[string]any{didPath: didWeb})},
		{name: "did:key skips the default", answers: map[string]any{"domain.base": domain, "did.method": "did:key"},
			want: scorpio},
		{name: "a given DID overrides the default", answers: map[string]any{"domain.base": domain, "did.value": "did:web:id.other.org"},
			want: merge(scorpio, map[string]any{didPath: "did:web:id.other.org"})},
		{name: "a given did:key DID passes its prefix", answers: map[string]any{"did.method": "did:key", "did.value": didKey},
			want: map[string]any{didPath: didKey}},
		{name: "a DID of another method fails its prefix", answers: map[string]any{"did.method": "did:key", "did.value": didWeb},
			wantErr: `did.value must start with "did:key:", which did.method sets`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := renderDID(t, tc.answers)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := values(tree); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("values = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRenderPrefixRefsDoNotGateDefault(t *testing.T) {
	tests := []struct {
		name    string
		answers map[string]any
		want    map[string]any
		wantErr string
	}{
		{name: "an unanswered prefix ref leaves the default derived and unchecked",
			answers: map[string]any{"tag.code": "a"}, want: map[string]any{didPath: "x-a"}},
		{name: "an answered prefix ref checks the derived value",
			answers: map[string]any{"tag.code": "a", "tag.scheme": "x-"}, want: map[string]any{didPath: "x-a"}},
		{name: "a derived value against its prefix fails",
			answers: map[string]any{"tag.code": "a", "tag.scheme": "y-"}, wantErr: `tag.value must start with "y-"`},
		{name: "a prefix ref alone derives nothing", answers: map[string]any{"tag.scheme": "x-"}, want: map[string]any{}},
	}
	b := modelFixture(t, prefixFixture)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := Render(b, profile.Profile{Answers: tc.answers}, Options{})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := values(tree); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("values = %v, want %v", got, tc.want)
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

// scorpioHostsPinned is the fixture's scorpio hosts list pinned whole without kept values.
var scorpioHostsPinned = map[string]pinWant{
	"scorpio.ingress.hosts[0].host":     {rendered: "scorpio." + domain},
	"scorpio.ingress.hosts[0].paths[0]": {rendered: "/"},
	"scorpio.ingress.hosts[1].host":     {rendered: scorpioAlt},
	"scorpio.ingress.hosts[1].paths[0]": {rendered: "/"},
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
			name: "an empty file pins nothing",
			body: "{}\n",
		},
		{
			name: "an empty mapping above a target pins nothing",
			body: "marketplace: {}\n",
		},
		{
			name: "a scalar element of an owned list pins the list whole",
			body: "scorpio:\n  ingress:\n    hosts:\n      - x\n",
			want: scorpioHostsPinned,
		},
		{
			name: "an empty owned list pins the list whole",
			body: "scorpio:\n  ingress:\n    hosts: []\n",
			want: scorpioHostsPinned,
		},
		{
			name: "a key no target writes inside an owned list pins the list whole",
			body: "keycloak:\n  ingress:\n    tls:\n      - foo: x\n",
			want: map[string]pinWant{
				"keycloak.ingress.tls[0].hosts[0]":   {rendered: "keycloak." + domain},
				"keycloak.ingress.tls[0].secretName": {rendered: "keycloak." + domain + "-tls"},
			},
		},
		{
			name: "a shadowed duplicate is pinned and keeps the last occurrence",
			body: "rainbow:\n  ingress:\n    host: rainbow.a.org\n    host: rainbow.b.org\n",
			want: map[string]pinWant{"rainbow.ingress.host": {value: "rainbow.b.org", kept: true, rendered: "rainbow." + domain}},
		},
	}
	answers := map[string]any{"org.displayName": displayName, "domain.base": domain, "host.scorpioAlt": scorpioAlt}
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
				seen := map[string]bool{}
				for _, l := range tree.Leaves {
					key := l.Path.String()
					seen[key] = true
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
				for key := range tc.want {
					if !seen[key] {
						t.Errorf("%s is missing from the tree", key)
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
	tests := []struct {
		name string
		body string
		key  string
		want any
	}{
		{"a collection at a target path", "marketplace:\n  title:\n    a: 1\n", "marketplace", map[string]any{"url": "https://market." + domain}},
		{"a scalar element of an owned list", "scorpio:\n  ingress:\n    hosts:\n      - x\n", "scorpio", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := load.Load([]byte(tc.body), "values.yaml", fixtureBundles(t), load.Options{})
			if err != nil {
				t.Fatal(err)
			}
			answers := map[string]any{"org.displayName": displayName, "domain.base": domain, "host.scorpioAlt": scorpioAlt}
			tree := renderFixture(t, answers, Options{Overrides: r})
			if got := tree.Nested()[tc.key]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s = %v, want %v", tc.key, got, tc.want)
			}
		})
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

// TestRenderTIRAddress renders the data space's TIR address into the verifier key only when answered, and keeps a
// loaded address unpinned once the load's derived answer is seeded.
func TestRenderTIRAddress(t *testing.T) {
	const (
		tirPath = "decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.tirAddress"
		loaded  = "decentralizedIam:\n  vcAuthentication:\n    vcverifier:\n      deployment:\n        verifier:\n" +
			"          tirAddress: " + referenceTIR + "\n"
	)
	b := providerBundle(t)
	res, err := load.Load([]byte(loaded), "values.yaml", []model.Bundle{b}, load.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		answers map[string]any
		opts    Options
		want    any
	}{
		{"unanswered", map[string]any{}, Options{}, nil},
		{"answered", map[string]any{tirAddressID: referenceTIR}, Options{}, referenceTIR},
		{"loaded and seeded", load.SeedAnswers(res.Report, nil), Options{Overrides: res}, referenceTIR},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := Render(b, profile.Profile{Answers: tc.answers}, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := values(tree)[tirPath]
			switch {
			case tc.want == nil && ok:
				t.Errorf("%s = %v, want no key", tirPath, got)
			case tc.want != nil && got != tc.want:
				t.Errorf("%s = %v, want %v", tirPath, got, tc.want)
			case tc.want != nil && leaf(t, tree, tirPath).Provenance.Pinned:
				t.Errorf("%s is pinned, want it re-rendered", tirPath)
			}
		})
	}
}

func TestRenderShippedVerifierAndDIDHosts(t *testing.T) {
	const pinnedDIDHost = "did." + domain
	b := providerBundle(t)
	tests := []struct {
		name    string
		answers map[string]any
		want    map[string]any
	}{
		{"derived from the domain", map[string]any{domainBase: domain}, map[string]any{
			"decentralizedIam.vcAuthentication.vcverifier.ingress.hosts[0].host":  "verifier." + domain,
			"decentralizedIam.vcAuthentication.vcverifier.deployment.server.host": "https://verifier." + domain,
			"marketplace.siop.verifier.host":                                      "https://verifier." + domain,
			"did.ingress.hosts[0].host":                                           domain,
			"did.config.server.hostUrl":                                           "http://" + domain,
			participantIDKey:                                                      didWebMethod + ":" + domain,
		}},
		{"a pinned DID host re-derives the DID", map[string]any{domainBase: domain, hostDIDID: pinnedDIDHost}, map[string]any{
			"did.ingress.hosts[0].host": pinnedDIDHost,
			"did.config.server.hostUrl": "http://" + pinnedDIDHost,
			participantIDKey:            didWebMethod + ":" + pinnedDIDHost,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := values(renderProvider(t, b, tc.answers))
			for path, want := range tc.want {
				if got[path] != want {
					t.Errorf("%s = %v, want %v", path, got[path], want)
				}
			}
		})
	}
}

const (
	resourceProfileID = "resources.profile"
	devProfile        = "development"
	prodProfile       = "production"
	customProfile     = "custom"
	// resourceLeaves is what a sized profile writes: four quantities on each of the table's five paths.
	resourceLeaves = 20
)

// resourceFigures are the shipped figures per table path: development and production requests cpu/memory,
// limits cpu/memory.
var resourceFigures = map[string][2][4]string{
	"keycloak.resources":                       {{"100m", "768Mi", "1", "2Gi"}, {"500m", "2Gi", "2", "2Gi"}},
	"scorpio.resources":                        {{"100m", "512Mi", "1500m", "1Gi"}, {"250m", "1Gi", "2", "1Gi"}},
	"tm-forum-api.defaultConfig.resources":     {{"100m", "512Mi", "1", "1Gi"}, {"250m", "1Gi", "2", "1Gi"}},
	"contract-management.deployment.resources": {{"50m", "256Mi", "500m", "512Mi"}, {"100m", "512Mi", "1", "512Mi"}},
	"decentralizedIam.vcAuthentication.trusted-issuers-list.deployment.resources": {
		{"50m", "256Mi", "500m", "512Mi"}, {"100m", "512Mi", "1", "512Mi"}},
}

// wantResources expands resourceFigures for a profile into leaf values by path; empty for custom.
func wantResources(profileName string) map[string]any {
	want := map[string]any{}
	row := map[string]int{devProfile: 0, prodProfile: 1}
	i, sized := row[profileName]
	if !sized {
		return want
	}
	leaves := []string{".requests.cpu", ".requests.memory", ".limits.cpu", ".limits.memory"}
	for path, figures := range resourceFigures {
		for j, l := range leaves {
			want[path+l] = figures[i][j]
		}
	}
	return want
}

// tableLeaves returns the tree's leaves under a resources table path, by path.
func tableLeaves(tree Tree) map[string]Leaf {
	got := map[string]Leaf{}
	for _, l := range tree.Leaves {
		key := l.Path.String()
		for path := range resourceFigures {
			if isUnder(key, path) || key == path {
				got[key] = l
			}
		}
	}
	return got
}

func TestRenderResourceProfile(t *testing.T) {
	b := providerBundle(t)
	tests := []struct {
		name    string
		env     profile.Environment
		answer  string
		profile string
	}{
		{"production renders the production figures", "", prodProfile, prodProfile},
		{"development renders the development figures", "", devProfile, devProfile},
		{"custom renders nothing", profile.EnvProduction, customProfile, customProfile},
		{"a production environment derives production", profile.EnvProduction, "", prodProfile},
		{"a development environment derives development", profile.EnvDevelopment, "", devProfile},
		{"no environment and no answer render nothing", "", "", customProfile},
		{"a given answer beats the environment", profile.EnvProduction, devProfile, devProfile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := profile.Profile{Target: profile.Target{Environment: tc.env}, Answers: map[string]any{}}
			if tc.answer != "" {
				p.Answers[resourceProfileID] = tc.answer
			}
			tree, err := Render(b, p, Options{})
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]any{}
			for key, l := range tableLeaves(tree) {
				got[key] = l.Value
				if want := []string{resourceProfileID}; !reflect.DeepEqual(l.Provenance.Answers, want) {
					t.Errorf("%s answers = %v, want %v", key, l.Provenance.Answers, want)
				}
			}
			want := wantResources(tc.profile)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("table leaves = %v, want %v", got, want)
			}
			if len(want) != 0 && len(got) != resourceLeaves {
				t.Errorf("%d table leaves, want %d", len(got), resourceLeaves)
			}
		})
	}
}

// TestRenderResourceProfileLeavesNoReservedRef keeps target.environment out of the answers.
func TestRenderResourceProfileLeavesNoReservedRef(t *testing.T) {
	got, err := Answers(providerBundle(t), profile.Profile{Target: profile.Target{Environment: profile.EnvProduction}})
	if err != nil {
		t.Fatal(err)
	}
	if got[resourceProfileID] != prodProfile {
		t.Errorf("%s = %v, want %s", resourceProfileID, got[resourceProfileID], prodProfile)
	}
	for id := range got {
		if strings.HasPrefix(id, "target") {
			t.Errorf("Answers holds %s, want no reserved ref", id)
		}
	}
}

func TestResourceProfileTargets(t *testing.T) {
	refs := Targets(providerBundle(t), resourceProfileID)
	got := map[string]bool{}
	components := map[string]bool{}
	for _, r := range refs {
		got[r.Path.String()] = true
		components[r.Component] = true
	}
	if len(refs) != resourceLeaves || len(got) != resourceLeaves {
		t.Errorf("%d targets on %d paths, want %d each once", len(refs), len(got), resourceLeaves)
	}
	for path := range wantResources(prodProfile) {
		if !got[path] {
			t.Errorf("%s is not a target", path)
		}
	}
	if want := len(resourceFigures); len(components) != want {
		t.Errorf("components = %v, want %d", components, want)
	}
}

func TestRenderResourceProfileKeepsHandEdit(t *testing.T) {
	const (
		path   = "scorpio.resources.limits.memory"
		edited = "3Gi"
	)
	b := providerBundle(t)
	r, err := load.Load([]byte("scorpio:\n  resources:\n    limits:\n      memory: "+edited+"\n"), "values.yaml", []model.Bundle{b}, load.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := Render(b, profile.Profile{Answers: map[string]any{resourceProfileID: prodProfile}}, Options{Overrides: r})
	if err != nil {
		t.Fatal(err)
	}
	for key, l := range tableLeaves(tree) {
		p := l.Provenance
		switch {
		case key == path && (!p.Pinned || !p.Kept || l.Value != edited || p.Rendered != wantResources(prodProfile)[path]):
			t.Errorf("%s = (value %v, pinned %v, kept %v, rendered %v), want %s kept over the rendered figure", key, l.Value, p.Pinned, p.Kept, p.Rendered, edited)
		case key != path && p.Pinned:
			t.Errorf("%s is pinned, want it rendered", key)
		}
	}
}

func TestDiffResourceProfileSwitch(t *testing.T) {
	b := providerBundle(t)
	render := func(name string) Tree {
		tree, err := Render(b, profile.Profile{Answers: map[string]any{resourceProfileID: name}}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		return tree
	}
	dev, prod := wantResources(devProfile), wantResources(prodProfile)
	want := []change{}
	for key := range prod {
		if dev[key] != prod[key] {
			want = append(want, change{foundation: resourceProfileID, path: key, status: StatusChanged, before: prod[key], after: dev[key]})
		}
	}
	got := flatten(Diff(b, render(prodProfile), render(devProfile)))
	if len(got) != len(want) {
		t.Fatalf("diff = %v, want %d changes", got, len(want))
	}
	for _, c := range got {
		if !slices.Contains(want, c) {
			t.Errorf("unexpected change %+v", c)
		}
	}
}
