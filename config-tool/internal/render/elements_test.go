package render

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"

	"gopkg.in/yaml.v3"
)

const (
	elementsFixture = "testdata/elements.yaml"
	caSecret        = "my-ca"
	httpsProxy      = "http://proxy:3128"
	release         = "provider"
	namespace       = "dsc"
	caMountPath     = "/etc/ssl/cert.pem"
)

var (
	heapEnv    = map[string]any{"name": "KC_HEAP_SIZE", "value": "1g"}
	realmMount = map[string]any{"name": "realms", "mountPath": "/opt/keycloak/data/import"}
	proxyEnv   = map[string]any{"name": "HTTPS_PROXY", "value": httpsProxy}
	trustEnv   = map[string]any{"name": "KC_TRUSTSTORE_PATHS", "value": "/opt/keycloak/conf/truststores/ca.crt"}
	trustMount = map[string]any{"name": "ca-bundle", "mountPath": "/opt/keycloak/conf/truststores"}
	caVolume   = map[string]any{"name": "ca-bundle", "secret": map[string]any{"secretName": caSecret}}
	caMount    = map[string]any{"name": "ca-bundle", "mountPath": caMountPath, "subPath": "ca.crt"}
	caAnswers  = map[string]any{"ca.secret": caSecret}
)

// renderElements renders the elements fixture over answers, with src as the loaded file when it is not empty.
func renderElements(t *testing.T, answers map[string]any, src string) Tree {
	t.Helper()
	b := modelFixture(t, elementsFixture)
	var opts Options
	if src != "" {
		root := roundtrip.Path{}
		r, err := load.Load([]byte(src), "values.yaml", []model.Bundle{b}, load.Options{Root: &root, Bundle: &b})
		if err != nil {
			t.Fatal(err)
		}
		opts.Overrides = r
	}
	p := profile.Profile{Answers: answers, Target: profile.Target{ReleaseName: release, Namespace: namespace}}
	tree, err := Render(b, p, opts)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// at returns the nested value at the dotted path p, nil when absent.
func at(t *testing.T, tree Tree, p string) any {
	t.Helper()
	path, err := roundtrip.ParsePath(p)
	if err != nil {
		t.Fatal(err)
	}
	return subtree(tree.Nested(), path)
}

func TestKeyedElements(t *testing.T) {
	both := map[string]any{"ca.secret": caSecret, "proxy.https": httpsProxy}
	tests := []struct {
		name    string
		answers map[string]any
		loaded  string
		want    map[string]any
	}{
		{
			name:    "base kept, owned appended in bundle order",
			answers: both,
			want: map[string]any{
				"keycloak.extraEnvVars":      []any{heapEnv, proxyEnv, trustEnv},
				"keycloak.extraVolumeMounts": []any{realmMount, trustMount},
				"verifier.volumes":           []any{caVolume},
				"verifier.mounts":            []any{caMount},
			},
		},
		{
			name:    "no answer writes no list, not even the base",
			answers: map[string]any{},
			want:    map[string]any{"keycloak.extraEnvVars": nil, "keycloak.extraVolumeMounts": nil, "verifier.volumes": nil},
		},
		{
			name:    "loaded list replaces the base; foreign elements untouched, owned replaced in place",
			answers: both,
			loaded: "keycloak:\n  extraEnvVars:\n    - {name: FOO, value: bar}\n" +
				"    - {name: KC_TRUSTSTORE_PATHS, value: /opt/keycloak/conf/truststores/ca.crt}\n    - {name: BAZ, value: qux}\n",
			want: map[string]any{
				"keycloak.extraEnvVars": []any{
					map[string]any{"name": "FOO", "value": "bar"}, trustEnv, map[string]any{"name": "BAZ", "value": "qux"}, proxyEnv,
				},
				"keycloak.extraVolumeMounts": []any{realmMount, trustMount},
			},
		},
		{
			name:    "loaded owned element with other content is kept as a hand edit",
			answers: caAnswers,
			loaded:  "keycloak:\n  extraEnvVars:\n    - {name: KC_TRUSTSTORE_PATHS, value: /elsewhere}\n",
			want: map[string]any{
				"keycloak.extraEnvVars": []any{map[string]any{"name": "KC_TRUSTSTORE_PATHS", "value": "/elsewhere"}},
			},
		},
		{
			name:    "loaded null is an empty list: no base, owned elements only",
			answers: both,
			loaded:  "keycloak:\n  extraEnvVars:\n  extraVolumeMounts: null\nverifier:\n  mounts: ~\n",
			want: map[string]any{
				"keycloak.extraEnvVars":      []any{proxyEnv, trustEnv},
				"keycloak.extraVolumeMounts": []any{trustMount},
				"verifier.volumes":           []any{caVolume},
				"verifier.mounts":            []any{caMount},
			},
		},
		{
			name:    "foreign mount with another mountPath does not collide",
			answers: caAnswers,
			loaded:  "verifier:\n  mounts:\n    - {name: root-ca, mountPath: /etc/other}\n",
			want: map[string]any{
				"verifier.mounts":  []any{map[string]any{"name": "root-ca", "mountPath": "/etc/other"}, caMount},
				"verifier.volumes": []any{caVolume},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderElements(t, tc.answers, tc.loaded)
			for p, want := range tc.want {
				if got := at(t, tree, p); !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %v, want %v", p, got, want)
				}
			}
			if len(tree.Withheld) != 0 {
				t.Errorf("Withheld = %v, want none", tree.Withheld)
			}
		})
	}
}

func TestKeyedElementProvenance(t *testing.T) {
	tree := renderElements(t, caAnswers, "keycloak:\n  extraEnvVars:\n    - {name: KC_TRUSTSTORE_PATHS, value: /elsewhere}\n")
	got := map[string]Provenance{}
	for _, l := range tree.Leaves {
		got[l.Path.String()] = l.Provenance
	}
	tests := []struct {
		path string
		want Provenance
	}{
		{"keycloak.extraVolumeMounts[0].name", Provenance{Foundation: "ca.secret", Answers: []string{}, Element: "realms", Origin: OriginBase}},
		{"keycloak.extraVolumeMounts[1].mountPath", Provenance{Foundation: "ca.secret", Answers: []string{}, Target: 2, Element: "ca-bundle"}},
		{"verifier.volumes[0].secret.secretName", Provenance{Foundation: "ca.secret", Answers: []string{"ca.secret"}, Target: 0, Element: "ca-bundle"}},
		{"keycloak.extraEnvVars[0].value", Provenance{
			Foundation: "ca.secret", Answers: []string{}, Target: 3, Element: "KC_TRUSTSTORE_PATHS", Pinned: true, Kept: true, Rendered: trustEnv,
		}},
	}
	for _, tc := range tests {
		if p := got[tc.path]; !reflect.DeepEqual(p, tc.want) {
			t.Errorf("%s provenance = %+v, want %+v", tc.path, p, tc.want)
		}
	}
}

func TestEditedListWithholds(t *testing.T) {
	edited := func(mechanism, element, list string) Withheld {
		return Withheld{Reason: WithheldEdited, Mechanism: mechanism, Element: element, List: list}
	}
	tests := []struct {
		name     string
		answers  map[string]any
		loaded   string
		want     map[string]any
		withheld []Withheld
	}{
		{
			name:     "scalar withholds the mechanism, its other list stays unwritten",
			answers:  caAnswers,
			loaded:   "verifier:\n  mounts: hand-written\n",
			want:     map[string]any{"verifier.volumes": nil, "verifier.mounts": nil, "keycloak.extraVolumeMounts": []any{realmMount, trustMount}},
			withheld: []Withheld{edited("verifier-ca", "ca-bundle", "verifier.mounts")},
		},
		{
			name:     "mapping withholds the mechanism",
			answers:  caAnswers,
			loaded:   "verifier:\n  volumes: {ca-bundle: {}}\n",
			want:     map[string]any{"verifier.volumes": nil, "verifier.mounts": nil},
			withheld: []Withheld{edited("verifier-ca", "ca-bundle", "verifier.volumes")},
		},
		{
			name:    "every element written there is withheld, with or without a mechanism",
			answers: map[string]any{"ca.secret": caSecret, "proxy.https": httpsProxy},
			loaded:  "keycloak:\n  extraEnvVars: hand-written\n",
			want: map[string]any{
				"keycloak.extraVolumeMounts": nil, "keycloak.proxy.httpsProxy": httpsProxy, "verifier.mounts": []any{caMount},
			},
			withheld: []Withheld{edited("", "HTTPS_PROXY", "keycloak.extraEnvVars"), edited("keycloak-ca", "KC_TRUSTSTORE_PATHS", "keycloak.extraEnvVars")},
		},
		{
			name:    "an unready element in an edited list is not reported",
			answers: map[string]any{"proxy.https": httpsProxy},
			loaded:  "verifier:\n  mounts: hand-written\n",
			want:    map[string]any{"verifier.mounts": nil, "keycloak.extraEnvVars": []any{heapEnv, proxyEnv}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderElements(t, tc.answers, tc.loaded)
			for p, want := range tc.want {
				if got := at(t, tree, p); !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %v, want %v", p, got, want)
				}
			}
			if !reflect.DeepEqual(tree.Withheld, tc.withheld) {
				t.Errorf("Withheld = %+v, want %+v", tree.Withheld, tc.withheld)
			}
			for _, w := range tc.withheld {
				for _, l := range tree.Leaves {
					if p, _ := roundtrip.ParsePath(w.List); len(l.Path) > len(p) && slices.Equal(l.Path[:len(p)], p) {
						t.Errorf("edited list %s got leaf %s", w.List, l.Path)
					}
				}
			}
		})
	}
}

func TestWithheldJSON(t *testing.T) {
	tests := []struct {
		name     string
		withheld Withheld
		want     string
	}{
		{
			name:     "edited carries no collision fields",
			withheld: Withheld{Reason: WithheldEdited, Mechanism: "verifier-ca", Element: "ca-bundle", List: "verifier.mounts"},
			want:     `{"reason":"edited","mechanism":"verifier-ca","element":"ca-bundle","list":"verifier.mounts"}`,
		},
		{
			name: "collision carries field, value and foreign key",
			withheld: Withheld{
				Reason: WithheldCollision, Mechanism: "verifier-ca", Element: "ca-bundle", List: "verifier.mounts", Field: "mountPath", Value: caMountPath, ForeignKey: "root-ca",
			},
			want: `{"reason":"collision","mechanism":"verifier-ca","element":"ca-bundle","list":"verifier.mounts","field":"mountPath","value":"` + caMountPath + `","foreignKey":"root-ca"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.withheld)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("json = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestMechanismWithheld(t *testing.T) {
	tree := renderElements(t, caAnswers, "verifier:\n  mounts:\n    - {name: root-ca, mountPath: "+caMountPath+"}\n")
	want := []Withheld{{
		Reason: WithheldCollision, Mechanism: "verifier-ca", Element: "ca-bundle", List: "verifier.mounts", Field: "mountPath", Value: caMountPath, ForeignKey: "root-ca",
	}}
	if !reflect.DeepEqual(tree.Withheld, want) {
		t.Errorf("Withheld = %+v, want %+v", tree.Withheld, want)
	}
	// The foreign mount stays as loaded because no leaf of either verifier list is written.
	for _, l := range tree.Leaves {
		if l.Path[0] == roundtrip.Key("verifier") {
			t.Errorf("withheld mechanism wrote %s", l.Path)
		}
	}
	if got := at(t, tree, "keycloak.extraVolumeMounts"); !reflect.DeepEqual(got, []any{realmMount, trustMount}) {
		t.Errorf("other mechanism: extraVolumeMounts = %v", got)
	}
}

func TestMechanismInterplay(t *testing.T) {
	const extraFile = "extra.crt"
	rootCA := map[string]any{"name": "root-ca", "mountPath": caMountPath}
	extraVolume := map[string]any{"name": "extra-ca", "secret": map[string]any{"secretName": caSecret}}
	extraMount := map[string]any{"name": "extra-ca", "mountPath": "/etc/extra", "subPath": extraFile}
	verifierCA := Withheld{Reason: WithheldCollision, Mechanism: "verifier-ca", Element: "ca-bundle", List: "verifier.mounts", Field: "mountPath", Value: caMountPath, ForeignKey: "root-ca"}
	both := map[string]any{"ca.secret": caSecret, "ca.extra": extraFile}
	tests := []struct {
		name     string
		answers  map[string]any
		loaded   string
		want     map[string]any
		withheld []Withheld
	}{
		{
			name:    "mechanism over two foundations with one answer renders nothing and is not withheld",
			answers: caAnswers,
			want:    map[string]any{"verifier.volumes": []any{caVolume}, "verifier.mounts": []any{caMount}},
		},
		{
			name:    "mechanism over two foundations with both answers renders",
			answers: both,
			want:    map[string]any{"verifier.volumes": []any{caVolume, extraVolume}, "verifier.mounts": []any{caMount, extraMount}},
		},
		{
			name:     "withheld mechanism's loaded owned element stays as foreign and withholds no other mechanism",
			answers:  both,
			loaded:   "verifier:\n  volumes:\n    - {name: ca-bundle, secret: {secretName: " + caSecret + "}}\n  mounts:\n    - {name: root-ca, mountPath: " + caMountPath + "}\n",
			want:     map[string]any{"verifier.volumes": []any{caVolume, extraVolume}, "verifier.mounts": []any{rootCA, extraMount}},
			withheld: []Withheld{verifierCA},
		},
		{
			name:    "two mechanisms withheld in one list leave it unwritten",
			answers: both,
			loaded:  "verifier:\n  mounts:\n    - {name: root-ca, mountPath: " + caMountPath + "}\n    - {name: other, mountPath: /etc/extra}\n",
			want:    map[string]any{"verifier.volumes": nil, "verifier.mounts": nil},
			withheld: []Withheld{
				{Reason: WithheldCollision, Mechanism: "extra-ca", Element: "extra-ca", List: "verifier.mounts", Field: "mountPath", Value: "/etc/extra", ForeignKey: "other"},
				verifierCA,
			},
		},
		{
			name:     "owned element pinned by a hand edit counts as foreign",
			answers:  both,
			loaded:   "verifier:\n  mounts:\n    - {name: ca-bundle, mountPath: /etc/extra}\n",
			want:     map[string]any{"verifier.mounts": []any{map[string]any{"name": "ca-bundle", "mountPath": "/etc/extra"}}},
			withheld: []Withheld{{Reason: WithheldCollision, Mechanism: "extra-ca", Element: "extra-ca", List: "verifier.mounts", Field: "mountPath", Value: "/etc/extra", ForeignKey: "ca-bundle"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderElements(t, tc.answers, tc.loaded)
			for p, want := range tc.want {
				if got := at(t, tree, p); !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %v, want %v", p, got, want)
				}
			}
			if !reflect.DeepEqual(tree.Withheld, tc.withheld) {
				t.Errorf("Withheld = %+v, want %+v", tree.Withheld, tc.withheld)
			}
		})
	}
}

func TestElementsDiff(t *testing.T) {
	b := modelFixture(t, elementsFixture)
	const rootCA = "verifier:\n  mounts:\n    - {name: root-ca, mountPath: " + caMountPath + "}\n"
	tests := []struct {
		name               string
		prev, next         map[string]any
		loaded             string
		wantStatus         Status
		wantPath, wantFrom string
		// withheld is set when verifier-ca is withheld, so no verifier leaf may change.
		withheld bool
	}{
		{name: "list written with its base", prev: map[string]any{}, next: caAnswers, wantStatus: StatusAdded,
			wantPath: "keycloak.extraVolumeMounts[0].name", wantFrom: "ca.secret"},
		{name: "list dropped with its base", prev: caAnswers, next: map[string]any{}, wantStatus: StatusRemoved,
			wantPath: "keycloak.extraEnvVars[0].value", wantFrom: "ca.secret"},
		{name: "index shift goes to the owner of the new element", prev: caAnswers, next: map[string]any{"ca.secret": caSecret, "proxy.https": httpsProxy},
			wantStatus: StatusChanged, wantPath: "keycloak.extraEnvVars[1].name", wantFrom: "proxy.https"},
		{name: "withheld mechanism", prev: map[string]any{}, next: caAnswers, loaded: rootCA, wantStatus: StatusAdded,
			wantPath: "keycloak.extraVolumeMounts[0].name", wantFrom: "ca.secret", withheld: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Diff(b, renderElements(t, tc.prev, tc.loaded), renderElements(t, tc.next, tc.loaded))
			found := false
			for _, g := range d.Groups {
				if g.Foundation == "" {
					t.Errorf("group without a foundation: %+v", g.Changes)
				}
				for _, c := range g.Changes {
					if tc.withheld && c.Path[0] == roundtrip.Key("verifier") {
						t.Errorf("%s in %s: withheld mechanism in the diff", c.Path, g.Foundation)
					}
					if c.Path.String() == tc.wantPath {
						found = true
						if c.Status != tc.wantStatus || g.Foundation != tc.wantFrom {
							t.Errorf("%s: %s in %q, want %s in %q", c.Path, c.Status, g.Foundation, tc.wantStatus, tc.wantFrom)
						}
					}
				}
			}
			if !found {
				t.Errorf("no change at %s in %+v", tc.wantPath, d.Groups)
			}
		})
	}
}

// TestElementTargets pins the fan-out of targets reached through requires or a mechanism, each list once.
func TestElementTargets(t *testing.T) {
	tests := []struct {
		answer    string
		wantPaths []string
	}{
		// keycloak.proxy.enabled is a const reached only through requires.
		{answer: "proxy.https", wantPaths: []string{
			"keycloak.extraEnvVars", "keycloak.proxy.enabled", "keycloak.proxy.httpsProxy", "keycloak.proxy.noProxy",
		}},
		// apisix.routes is an each const, and both keycloak-ca elements hold literals only.
		{answer: "ca.secret", wantPaths: []string{
			"apisix.routes", "keycloak.extraEnvVars", "keycloak.extraVolumeMounts", "verifier.mounts", "verifier.volumes",
		}},
		// extra-ca's volume references only ca.secret but renders with its mechanism.
		{answer: "ca.extra", wantPaths: []string{"verifier.mounts", "verifier.volumes"}},
		{answer: "proxy.no", wantPaths: []string{"keycloak.proxy.noProxy"}},
	}
	b := modelFixture(t, elementsFixture)
	for _, tc := range tests {
		t.Run(tc.answer, func(t *testing.T) {
			var paths []string
			for _, r := range Targets(b, tc.answer) {
				paths = append(paths, r.Path.String())
			}
			if !slices.Equal(paths, tc.wantPaths) {
				t.Errorf("targets = %v, want %v", paths, tc.wantPaths)
			}
		})
	}
}

func TestEachTargets(t *testing.T) {
	const verify = "plugins.openid-connect.ssl_verify"
	tests := []struct {
		name   string
		loaded string
		want   map[string]any
		pinned []string
	}{
		{name: "no loaded file", want: map[string]any{"apisix.routes": nil}},
		{name: "zero routes", loaded: "apisix:\n  routes: []\n", want: map[string]any{"apisix.routes": nil}},
		{
			name:   "one route",
			loaded: "apisix:\n  routes:\n    - plugins:\n        openid-connect: {discovery: d}\n",
			want:   map[string]any{"apisix.routes[0]." + verify: true},
		},
		{
			name: "many routes; one without the parent, one set false by hand",
			loaded: "apisix:\n  routes:\n    - plugins:\n        openid-connect: {ssl_verify: true}\n" +
				"    - plugins:\n        opa: {}\n    - plugins:\n        openid-connect: {ssl_verify: false}\n",
			want:   map[string]any{"apisix.routes[0]." + verify: true, "apisix.routes[1]": nil, "apisix.routes[2]." + verify: false},
			pinned: []string{"apisix.routes[2]." + verify},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderElements(t, caAnswers, tc.loaded)
			for p, want := range tc.want {
				if got := at(t, tree, p); !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %v, want %v", p, got, want)
				}
			}
			var pinned []string
			for _, l := range tree.Leaves {
				if l.Provenance.Pinned {
					pinned = append(pinned, l.Path.String())
				}
			}
			if !reflect.DeepEqual(pinned, tc.pinned) {
				t.Errorf("pinned = %v, want %v", pinned, tc.pinned)
			}
		})
	}
	if got := at(t, renderElements(t, nil, "apisix:\n  routes:\n    - plugins:\n        openid-connect: {}\n"), "apisix.routes"); got != nil {
		t.Errorf("unanswered each target wrote %v", got)
	}
}

func TestRequiresAndDefaultWhenSet(t *testing.T) {
	const noProxy = "localhost," + namespace + ".svc," + release + "-keycloak"
	tests := []struct {
		name        string
		answers     map[string]any
		wantNo      any
		wantEnabled any
	}{
		{name: "nothing given: proxy.no not derived though its refs resolve", answers: map[string]any{}},
		{name: "given proxy.no without proxy.https renders nothing", answers: map[string]any{"proxy.no": "x"}},
		{name: "proxy.https derives proxy.no", answers: map[string]any{"proxy.https": httpsProxy}, wantNo: noProxy, wantEnabled: true},
		{name: "given proxy.no wins", answers: map[string]any{"proxy.https": httpsProxy, "proxy.no": "x"}, wantNo: "x", wantEnabled: true},
	}
	b := modelFixture(t, elementsFixture)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderElements(t, tc.answers, "")
			if got := at(t, tree, "keycloak.proxy.noProxy"); got != tc.wantNo {
				t.Errorf("noProxy = %v, want %v", got, tc.wantNo)
			}
			if got := at(t, tree, "keycloak.proxy.enabled"); got != tc.wantEnabled {
				t.Errorf("enabled = %v, want %v", got, tc.wantEnabled)
			}
			p := profile.Profile{Answers: tc.answers, Target: profile.Target{ReleaseName: release, Namespace: namespace}}
			answers, err := Answers(b, p)
			if err != nil {
				t.Fatal(err)
			}
			if _, derived := answers["proxy.no"]; derived != (tc.wantNo != nil || tc.answers["proxy.no"] != nil) {
				t.Errorf("Answers has proxy.no = %v", derived)
			}
		})
	}
}

// TestLoadRenderRoundTrip loads a rendered tree and renders it again from the answers the load derived: the trees
// must be equal and no list leaf may be pinned.
func TestLoadRenderRoundTrip(t *testing.T) {
	const foreign = "keycloak:\n  extraEnvVars:\n    - {name: NODE_TLS_REJECT_UNAUTHORIZED, value: '0'}\n" +
		"apisix:\n  routes:\n    - plugins:\n        openid-connect: {discovery: d}\n    - plugins:\n        opa: {}\n"
	tests := []struct {
		name    string
		answers map[string]any
		loaded  string
	}{
		{name: "proxy", answers: map[string]any{"proxy.https": httpsProxy}},
		{name: "proxy and no proxy", answers: map[string]any{"proxy.https": httpsProxy, "proxy.no": "localhost,dsc.svc"}},
		{name: "private CA", answers: caAnswers},
		{name: "two mechanisms over two foundations", answers: map[string]any{"ca.secret": caSecret, "ca.extra": "x.crt", "proxy.https": httpsProxy}},
		{name: "among foreign elements", answers: map[string]any{"ca.secret": caSecret, "proxy.https": httpsProxy}, loaded: foreign},
	}
	b := modelFixture(t, elementsFixture)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			first := renderElements(t, tc.answers, tc.loaded)
			src, err := yaml.Marshal(first.Nested())
			if err != nil {
				t.Fatal(err)
			}
			root := roundtrip.Path{}
			r, err := load.Load(src, "values.yaml", []model.Bundle{b}, load.Options{Root: &root, Bundle: &b})
			if err != nil {
				t.Fatal(err)
			}
			p := profile.Profile{Answers: load.SeedAnswers(r.Report, nil), Target: profile.Target{ReleaseName: release, Namespace: namespace}}
			second, err := Render(b, p, Options{Overrides: r})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(second.Nested(), first.Nested()) {
				t.Errorf("round trip:\n got %v\nwant %v\nloaded\n%s", second.Nested(), first.Nested(), src)
			}
			// Scalar targets are owned only through owned markers (ADR 0020), which Marshal does not write.
			for _, l := range second.Leaves {
				if l.Provenance.Pinned && len(lists(l.Path)) > 0 {
					t.Errorf("%s pinned after a round trip", l.Path)
				}
			}
		})
	}
}

// TestLoadOwnedIsRendered checks that every list leaf the load reports as owned is one the render writes unpinned
// from the seeded answers, so the report never claims an element the render leaves foreign.
func TestLoadOwnedIsRendered(t *testing.T) {
	tests := []struct{ name, loaded string }{
		{name: "mount without its volume", loaded: "verifier:\n  mounts:\n    - {name: ca-bundle, mountPath: /etc/ssl/cert.pem, subPath: ca.crt}\n"},
		{name: "mechanism over two foundations, one missing", loaded: "verifier:\n  mounts:\n    - {name: extra-ca, mountPath: /etc/extra, subPath: x.crt}\n"},
		{name: "each without its requires", loaded: "apisix:\n  routes:\n    - plugins:\n        openid-connect: {ssl_verify: true}\n"},
		{name: "complete mechanism", loaded: "verifier:\n  volumes:\n    - {name: extra-ca, secret: {secretName: s}}\n" +
			"  mounts:\n    - {name: extra-ca, mountPath: /etc/extra, subPath: x.crt}\n"},
	}
	b := modelFixture(t, elementsFixture)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := roundtrip.Path{}
			r, err := load.Load([]byte(tc.loaded), "values.yaml", []model.Bundle{b}, load.Options{Root: &root, Bundle: &b})
			if err != nil {
				t.Fatal(err)
			}
			p := profile.Profile{Answers: load.SeedAnswers(r.Report, nil), Target: profile.Target{ReleaseName: release, Namespace: namespace}}
			tree, err := Render(b, p, Options{Overrides: r})
			if err != nil {
				t.Fatal(err)
			}
			rendered := map[string]bool{}
			for _, l := range tree.Leaves {
				rendered[l.Path.String()] = !l.Provenance.Pinned
			}
			for _, e := range r.Report.Owned {
				if !rendered[e.Path] {
					t.Errorf("%s is owned, but the render does not write it unpinned", e.Path)
				}
			}
		})
	}
}

// TestDiffPinnedElement reports a hand-edited owned element once, under its key, with the whole elements.
func TestDiffPinnedElement(t *testing.T) {
	const (
		before = "http://old:3128"
		edited = "keycloak:\n  extraEnvVars:\n    - {name: HTTPS_PROXY, value: '" + before + "', note: kept}\n"
	)
	b := modelFixture(t, elementsFixture)
	prev := renderElements(t, map[string]any{"proxy.https": before}, edited)
	next := renderElements(t, map[string]any{"proxy.https": httpsProxy}, edited)
	var got []Change
	for _, g := range Diff(b, prev, next).Groups {
		for _, c := range g.Changes {
			if c.Element != "" {
				if g.Foundation != "proxy.https" {
					t.Errorf("%s[%s] in %q, want proxy.https", c.Path, c.Element, g.Foundation)
				}
				got = append(got, c)
			}
		}
	}
	if len(got) != 1 {
		t.Fatalf("element changes = %+v, want one", got)
	}
	data, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"path":"keycloak.extraEnvVars[name=HTTPS_PROXY]","status":"pinned",` +
		`"before":{"name":"HTTPS_PROXY","value":"` + before + `"},"after":{"name":"HTTPS_PROXY","value":"` + httpsProxy + `"},` +
		`"kept":{"name":"HTTPS_PROXY","note":"kept","value":"` + before + `"}}`
	if string(data) != want {
		t.Errorf("change = %s\nwant     %s", data, want)
	}
}
