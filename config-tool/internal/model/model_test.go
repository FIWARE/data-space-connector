package model

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	"github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	chart       = "data-space-connector"
	trustAnchor = "trust-anchor"
)

func bundleFile(chart, version string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("chart: " + chart + "\nversion: " + version + "\nrevision: 1\n")}
}

func TestVersions(t *testing.T) {
	tests := []struct {
		name    string
		fs      fstest.MapFS
		want    []Bundle
		wantErr string
	}{
		{name: "no bundles", fs: fstest.MapFS{}, want: []Bundle{}},
		{
			name: "single bundle is latest",
			fs:   fstest.MapFS{"10.16.0/bundle.yaml": bundleFile(chart, "10.16.0")},
			want: []Bundle{{Chart: chart, Version: "10.16.0", Revision: 1, Latest: true}},
		},
		{
			name: "semver order, not lexical",
			fs: fstest.MapFS{
				"9.2.0/bundle.yaml":         bundleFile(chart, "9.2.0"),
				"10.16.0/bundle.yaml":       bundleFile(chart, "10.16.0"),
				"10.9.1/bundle.yaml":        bundleFile(chart, "10.9.1"),
				"10.16.0-rc.1/bundle.yaml":  bundleFile(chart, "10.16.0-rc.1"),
				"10.16.0/unrelated.yaml":    {Data: []byte("ignored")},
				"no-bundle-here/notes.yaml": {Data: []byte("ignored")},
			},
			want: []Bundle{
				{Chart: chart, Version: "10.16.0", Revision: 1, Latest: true},
				{Chart: chart, Version: "10.16.0-rc.1", Revision: 1},
				{Chart: chart, Version: "10.9.1", Revision: 1},
				{Chart: chart, Version: "9.2.0", Revision: 1},
			},
		},
		{
			name: "a newer pre-release is not latest",
			fs: fstest.MapFS{
				"10.16.0/bundle.yaml":      bundleFile(chart, "10.16.0"),
				"10.17.0-rc.1/bundle.yaml": bundleFile(chart, "10.17.0-rc.1"),
			},
			want: []Bundle{
				{Chart: chart, Version: "10.17.0-rc.1", Revision: 1},
				{Chart: chart, Version: "10.16.0", Revision: 1, Latest: true},
			},
		},
		{
			name: "only pre-releases: the highest is latest",
			fs: fstest.MapFS{
				"10.17.0-rc.1/bundle.yaml": bundleFile(chart, "10.17.0-rc.1"),
				"10.17.0-rc.2/bundle.yaml": bundleFile(chart, "10.17.0-rc.2"),
			},
			want: []Bundle{
				{Chart: chart, Version: "10.17.0-rc.2", Revision: 1, Latest: true},
				{Chart: chart, Version: "10.17.0-rc.1", Revision: 1},
			},
		},
		{
			name: "latest per chart",
			fs: fstest.MapFS{
				"10.16.0/bundle.yaml":    bundleFile(chart, "10.16.0"),
				"10.15.0/bundle.yaml":    bundleFile(chart, "10.15.0"),
				"0.2.0/bundle.yaml":      bundleFile(trustAnchor, "0.2.0"),
				"0.3.0-rc.1/bundle.yaml": bundleFile(trustAnchor, "0.3.0-rc.1"),
			},
			want: []Bundle{
				{Chart: chart, Version: "10.16.0", Revision: 1, Latest: true},
				{Chart: chart, Version: "10.15.0", Revision: 1},
				{Chart: trustAnchor, Version: "0.3.0-rc.1", Revision: 1},
				{Chart: trustAnchor, Version: "0.2.0", Revision: 1, Latest: true},
			},
		},
		{name: "version is not semver", fs: fstest.MapFS{"latest/bundle.yaml": bundleFile(chart, "latest")}, wantErr: "not semver"},
		{name: "major.minor only", fs: fstest.MapFS{"10.16/bundle.yaml": bundleFile(chart, "10.16")}, wantErr: "not semver"},
		{name: "major only", fs: fstest.MapFS{"10/bundle.yaml": bundleFile(chart, "10")}, wantErr: "not semver"},
		{name: "v prefix", fs: fstest.MapFS{"v10.16.0/bundle.yaml": bundleFile(chart, "v10.16.0")}, wantErr: "not semver"},
		{name: "version differs from directory", fs: fstest.MapFS{"10.16.0/bundle.yaml": bundleFile(chart, "10.15.0")}, wantErr: "does not match"},
		{name: "chart missing", fs: fstest.MapFS{"10.16.0/bundle.yaml": bundleFile("", "10.16.0")}, wantErr: "chart is missing"},
		{name: "revision missing", fs: fstest.MapFS{"10.16.0/bundle.yaml": {Data: []byte("chart: " + chart + "\nversion: 10.16.0\n")}}, wantErr: "revision is missing"},
		{name: "revision zero", fs: fstest.MapFS{"10.16.0/bundle.yaml": {Data: []byte("chart: " + chart + "\nversion: 10.16.0\nrevision: 0\n")}}, wantErr: "revision is missing"},
		{name: "not YAML", fs: fstest.MapFS{"10.16.0/bundle.yaml": {Data: []byte("chart: [")}}, wantErr: "10.16.0/bundle.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Versions(tc.fs)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Versions = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCharts(t *testing.T) {
	dsc10, dsc9 := Bundle{Chart: chart, Version: "10.16.0", Revision: 1, Latest: true}, Bundle{Chart: chart, Version: "9.2.0", Revision: 1}
	ta := Bundle{Chart: trustAnchor, Version: "0.2.0", Revision: 1, Latest: true}
	tests := []struct {
		name    string
		bundles []Bundle
		want    []Chart
	}{
		{"none", nil, []Chart{}},
		{
			"grouped by chart in first-seen order",
			[]Bundle{dsc10, ta, dsc9},
			[]Chart{{Name: chart, Versions: []Bundle{dsc10, dsc9}}, {Name: trustAnchor, Versions: []Bundle{ta}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Charts(tc.bundles); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Charts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestEmbeddedBundles keeps the shipped bundles loadable, so a bad bundle fails here and not at run time.
func TestEmbeddedBundles(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || !got[0].Latest || got[0].Chart != chart {
		t.Errorf("embedded bundles = %+v, want a latest %s bundle first", got, chart)
	}
}

// repoRoot is the repository root, relative to this package directory.
const (
	repoRoot    = "../../.."
	chartValues = repoRoot + "/charts/data-space-connector/values.yaml"
	fixtureDir  = "testdata/bundles"
	fixtureFile = "1.0.0/bundle.yaml"
)

func rulesBundle(rules string) fstest.MapFS {
	return fstest.MapFS{fixtureFile: {Data: []byte("chart: " + chart + "\nversion: 1.0.0\nrevision: 1\nrecognize:\n" + rules)}}
}

func TestRuleValidation(t *testing.T) {
	const toggle = "  - {path: did.enabled, answer: identity.did.enabled, stage: features, kind: toggle}\n"
	tests := []struct {
		name    string
		rules   string
		wantErr string
	}{
		{"path missing", "  - {answer: a, stage: features, kind: toggle}\n", "recognize[0]: path is missing"},
		{"path malformed", "  - {path: 'a..b', answer: a, stage: features, kind: toggle}\n", "recognize[0]: roundtrip: invalid path"},
		{"answer missing", "  - {path: a, stage: features, kind: toggle}\n", "recognize[0]: answer is missing"},
		{"stage unknown", "  - {path: a, answer: a, stage: nowhere, kind: toggle}\n", `stage "nowhere" is unknown`},
		{"stage missing", "  - {path: a, answer: a, kind: toggle}\n", `stage "" is unknown`},
		{"kind unknown", "  - {path: a, answer: a, stage: features, kind: number}\n", `kind "number" is unknown`},
		{"pattern on a toggle", "  - {path: a, answer: a, stage: features, kind: toggle, pattern: '(x)'}\n", "only allowed on value rules"},
		{"pattern does not compile", "  - {path: a, answer: a, stage: features, kind: value, pattern: '(x'}\n", "pattern: error parsing regexp"},
		{"pattern without group", "  - {path: a, answer: a, stage: features, kind: value, pattern: 'x'}\n", "has 0 capture groups, want 1"},
		{"pattern with two groups", "  - {path: a, answer: a, stage: features, kind: value, pattern: '(x)(y)'}\n", "has 2 capture groups, want 1"},
		{"duplicate path", toggle + toggle, "recognize[1]: path \"did.enabled\" is already used by recognize[0]"},
		{"answer with two stages", toggle + "  - {path: x.enabled, answer: identity.did.enabled, stage: configuration, kind: toggle}\n", `recognize[1]: answer "identity.did.enabled" has stage configuration, recognize[0] has features`},
		{"answer with two kinds", toggle + "  - {path: x.host, answer: identity.did.enabled, stage: features, kind: value}\n", `recognize[1]: answer "identity.did.enabled" has kind value, recognize[0] has toggle`},
		{"unknown rule field", "  - {path: a, answer: a, stage: features, kind: value, patern: '^[^.]+\\.(.+)$'}\n", "field patern not found"},
		{"duplicate path in another spelling", toggle + "  - {path: 'did.\"enabled\"', answer: b, stage: features, kind: toggle}\n", "already used by recognize[0]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Versions(rulesBundle(tc.rules))
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to name %s and mention %q", err, fixtureFile, tc.wantErr)
			}
		})
	}
}

func TestRules(t *testing.T) {
	got, err := Versions(os.DirFS(fixtureDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("fixture bundles = %+v, want one", got)
	}
	rules := got[0].Rules()
	if len(rules) != len(got[0].Recognize) {
		t.Fatalf("Rules() has %d entries, want %d", len(rules), len(got[0].Recognize))
	}
	for i, r := range rules {
		src := got[0].Recognize[i]
		if r.Path.String() != src.Path || r.Answer != src.Answer || r.Stage != src.Stage || r.Kind != src.Kind {
			t.Errorf("Rules()[%d] = %+v, want it compiled from %+v", i, r, src)
		}
		if (r.Pattern == nil) != (src.Pattern == "") {
			t.Errorf("Rules()[%d].Pattern = %v, want it compiled from %q", i, r.Pattern, src.Pattern)
		}
	}
	const host, domain = "keycloak.example.org", "example.org"
	for _, r := range rules {
		if r.Pattern == nil {
			continue
		}
		if m := r.Pattern.FindStringSubmatch(host); m == nil || m[1] != domain {
			t.Errorf("%s pattern on %q = %v, want %q", r.Path, host, m, domain)
		}
	}
}

// TestEmbeddedRulesResolve keeps every shipped rule pointing at a key the chart really has, so a
// renamed chart key breaks the build.
func TestEmbeddedRulesResolve(t *testing.T) {
	src, err := os.ReadFile(chartValues)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := roundtrip.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got {
		if b.Chart != chart {
			continue
		}
		for _, r := range b.Rules() {
			n, err := doc.Lookup(r.Path)
			if err != nil {
				t.Errorf("%s %s: %v", b.Version, r.Path, err)
				continue
			}
			if r.Kind == KindToggle && n.Tag != "!!bool" {
				t.Errorf("%s %s: toggle rule on a %s node, want !!bool", b.Version, r.Path, n.Tag)
			}
		}
	}
}

func TestTopLevelKeysValidation(t *testing.T) {
	const head = "chart: " + chart + "\nversion: 1.0.0\nrevision: 1\n"
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"empty key", "topLevelKeys: [did, '']\n", "topLevelKeys[1] is empty"},
		{"repeated key", "topLevelKeys: [did, vault, did]\n", `topLevelKeys[2]: "did" is already listed at topLevelKeys[0]`},
		{"rule outside the keys", "topLevelKeys: [vault]\nrecognize:\n  - {path: did.enabled, answer: a, stage: features, kind: toggle}\n", `recognize[0]: "did" is not in topLevelKeys`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Versions(fstest.MapFS{fixtureFile: {Data: []byte(head + tc.body)}})
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.wantErr)
			}
		})
	}
}

// TestEmbeddedTopLevelKeys keeps the latest bundle's topLevelKeys equal to the chart's, so a load
// still finds wrapped chart values after the chart gains or renames a section.
func TestEmbeddedTopLevelKeys(t *testing.T) {
	src, err := os.ReadFile(chartValues)
	if err != nil {
		t.Fatal(err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		t.Fatal(err)
	}
	var want []string
	values := root.Content[0].Content
	for i := 0; i < len(values); i += 2 {
		want = append(want, values[i].Value)
	}
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	keys := slices.Sorted(slices.Values(got[0].TopLevelKeys))
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Errorf("latest bundle topLevelKeys = %v, want the chart's %v", keys, want)
	}
}
