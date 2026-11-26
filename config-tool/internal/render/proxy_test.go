package render

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	umbrellaValues     = "../../../charts/data-space-connector/values.yaml"
	umbrellaChartFile  = "../../../charts/data-space-connector/Chart.yaml"
	subchartDefaults   = fixtureDir + "/subchart-defaults"
	proxyHTTPSID       = "proxy.https"
	proxyHTTPID        = "proxy.http"
	proxyNoID          = "proxy.no"
	verifierEnvVars    = "decentralizedIam.vcAuthentication.vcverifier.deployment.additionalEnvVars"
	logicProxyEnvVars  = "marketplace.bizEcosystemLogicProxy.additionalEnvVars"
	vcOperatorEnvVars  = "vc-operator.extraEnvVars"
	apisixProxyOpts    = "decentralizedIam.odrlAuthorization.apisix.routes[0].plugins.openid-connect.proxy_opts"
	keycloakProxy      = "keycloak.proxy"
	logicProxyBaseSize = 9
	// oneRoute is a loaded file with a single openid-connect route, so the APISIX each targets have an element.
	oneRoute = "decentralizedIam:\n  odrlAuthorization:\n    apisix:\n      routes:\n        - plugins:\n" +
		"            openid-connect: {discovery: https://verifier.example.org}\n"
	// noProxyForms are the entries of the proxy.no default before the bare service names, for namespace.
	noProxyForms = "localhost,127.0.0.1,svc,.svc,cluster.local,.cluster.local," + namespace + ".svc"
	// wantNoProxy is the proxy.no default for release and namespace.
	wantNoProxy = noProxyForms + ",credentials-config-service,trusted-issuers-list,tm-forum-api,tm-forum-api-svc,status-list-server," +
		release + "-keycloak," + release + "-opentelemetry-collector"
)

var proxyIDs = []string{proxyHTTPSID, proxyHTTPID, proxyNoID}

// proxyComponents are the first two path segments of each of the five components the proxy group reaches.
var proxyComponents = []string{"decentralizedIam.odrlAuthorization", "decentralizedIam.vcAuthentication", "keycloak.proxy",
	"marketplace.bizEcosystemLogicProxy", "vc-operator.extraEnvVars"}

// subchartFixture holds the defaults of one subchart version that a lists: base or a noProxyServices row reads.
type subchartFixture struct {
	Chart   string         `yaml:"chart"`
	Version string         `yaml:"version"`
	Values  map[string]any `yaml:"values"`
	// Names are service names the subchart's templates write literally.
	Names []string `yaml:"names"`
}

type chartDependency struct {
	Name    string `yaml:"name"`
	Alias   string `yaml:"alias"`
	Version string `yaml:"version"`
}

func chartDependencies(t *testing.T) []chartDependency {
	t.Helper()
	src, err := os.ReadFile(umbrellaChartFile)
	if err != nil {
		t.Fatal(err)
	}
	var chart struct {
		Dependencies []chartDependency `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(src, &chart); err != nil {
		t.Fatal(err)
	}
	return chart.Dependencies
}

// subchartFixtures returns the fixtures by the umbrella key their chart sits under, and fails on a fixture whose
// version is not the dependency version in Chart.yaml.
func subchartFixtures(t *testing.T) map[string]subchartFixture {
	t.Helper()
	files, err := filepath.Glob(subchartDefaults + "/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no subchart fixtures in %s: %v", subchartDefaults, err)
	}
	deps := chartDependencies(t)
	byKey := map[string]subchartFixture{}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var f subchartFixture
		if err := yaml.Unmarshal(src, &f); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if want := f.Chart + "-" + f.Version + ".yaml"; filepath.Base(file) != want {
			t.Errorf("%s holds %s %s; name it %s", file, f.Chart, f.Version, want)
		}
		i := slices.IndexFunc(deps, func(d chartDependency) bool { return d.Name == f.Chart })
		if i < 0 {
			t.Errorf("%s: %s is no dependency in Chart.yaml", file, f.Chart)
			continue
		}
		if d := deps[i]; d.Version != f.Version {
			t.Errorf("%s: Chart.yaml pins %s %s; pull the fixture again", file, f.Chart, d.Version)
		}
		key := deps[i].Alias
		if key == "" {
			key = deps[i].Name
		}
		byKey[key] = f
	}
	return byKey
}

// lookupValue returns the value at p below v, and whether every segment is there; a null leaf is found.
func lookupValue(v any, p roundtrip.Path) (any, bool) {
	for _, s := range p {
		k, ok := s.Key()
		m, isMap := v.(map[string]any)
		if !ok || !isMap {
			return nil, false
		}
		if v, ok = m[k]; !ok {
			return nil, false
		}
	}
	return v, true
}

// chartDefault returns the default at p from the layer that supplies it: the umbrella when it declares p, otherwise
// the subchart fixture. from names the layer; ok is false when neither declares p.
func chartDefault(t *testing.T, umbrella *roundtrip.Document, fixtures map[string]subchartFixture, p roundtrip.Path) (v any, from string, ok bool) {
	t.Helper()
	if n, err := umbrella.Lookup(p); err == nil {
		if err := n.Decode(&v); err != nil {
			t.Fatal(err)
		}
		return v, "umbrella", true
	}
	root, _ := p[0].Key()
	f, ok := fixtures[root]
	if !ok {
		return nil, "", false
	}
	v, ok = lookupValue(f.Values, p[1:])
	return v, f.Chart + " " + f.Version, ok
}

func TestSubchartFixtureVersions(t *testing.T) {
	subchartFixtures(t)
}

// TestListBases pins each lists: base against the layer that supplies the list's default, because Helm replaces the
// whole list once the tool writes it.
func TestListBases(t *testing.T) {
	umbrella, fixtures := parseValues(t, umbrellaValues), subchartFixtures(t)
	for _, l := range providerBundle(t).ListTable() {
		ref, from, ok := chartDefault(t, umbrella, fixtures, l.Path)
		if !ok {
			t.Errorf("lists %s: neither the umbrella nor a subchart fixture declares it", l.Path)
			continue
		}
		want, _ := ref.([]any)
		if ref != nil && want == nil {
			t.Errorf("lists %s: %s default is %#v, not a list", l.Path, from, ref)
			continue
		}
		var got []any
		for _, e := range l.Base {
			got = append(got, e)
		}
		if len(got) != len(want) || len(want) > 0 && !reflect.DeepEqual(got, want) {
			t.Errorf("lists %s base = %#v, %s default is %#v", l.Path, got, from, want)
		}
	}
}

// TestProxyTargetPaths pins the proxy fan-out: every target path is a key the umbrella or the subchart declares.
func TestProxyTargetPaths(t *testing.T) {
	b := providerBundle(t)
	umbrella, fixtures := parseValues(t, umbrellaValues), subchartFixtures(t)
	components := map[string]bool{}
	for _, id := range proxyIDs {
		f, ok := b.Foundation(id)
		if !ok {
			t.Fatalf("no foundation %s", id)
		}
		for i, tg := range f.Targets {
			if _, _, ok := chartDefault(t, umbrella, fixtures, tg.Path); !ok {
				t.Errorf("%s targets[%d] %s: neither the umbrella nor a subchart fixture declares it", id, i, tg.Path)
			}
			components[tg.Path[:2].String()] = true
		}
	}
	if got := slices.Sorted(maps.Keys(components)); !reflect.DeepEqual(got, proxyComponents) {
		t.Errorf("proxy targets reach %v, want the five components %v", got, proxyComponents)
	}
}

// renderShipped renders the shipped bundle for release in namespace, over src as the loaded file when not empty.
func renderShipped(t *testing.T, answers map[string]any, src string) Tree {
	t.Helper()
	return renderShippedFor(t, answers, src, profile.Target{ReleaseName: release, Namespace: namespace})
}

// renderShippedFor is renderShipped for target.
func renderShippedFor(t *testing.T, answers map[string]any, src string, target profile.Target) Tree {
	t.Helper()
	b := providerBundle(t)
	var opts Options
	if src != "" {
		root := roundtrip.Path{}
		r, err := load.Load([]byte(src), "values.yaml", []model.Bundle{b}, load.Options{Root: &root, Bundle: &b})
		if err != nil {
			t.Fatal(err)
		}
		opts.Overrides = r
	}
	tree, err := Render(b, profile.Profile{Answers: answers, Target: target}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func env(name, value string) map[string]any {
	return map[string]any{"name": name, "value": value}
}

func TestProxyRender(t *testing.T) {
	tree := renderShipped(t, map[string]any{proxyHTTPSID: httpsProxy}, oneRoute)
	proxyEnvs := []any{env("HTTPS_PROXY", httpsProxy), env("HTTP_PROXY", httpsProxy), env("NO_PROXY", wantNoProxy)}
	if got := at(t, tree, verifierEnvVars); !reflect.DeepEqual(got, proxyEnvs) {
		t.Errorf("verifier env = %#v, want %#v", got, proxyEnvs)
	}
	if got := at(t, tree, vcOperatorEnvVars); !reflect.DeepEqual(got, proxyEnvs) {
		t.Errorf("vc-operator env = %#v, want %#v", got, proxyEnvs)
	}
	lp, _ := at(t, tree, logicProxyEnvVars).([]any)
	if len(lp) != logicProxyBaseSize+len(proxyEnvs) || !reflect.DeepEqual(lp[logicProxyBaseSize:], proxyEnvs) {
		t.Errorf("logic proxy env = %#v, want the base and then %#v", lp, proxyEnvs)
	}
	wantKeycloak := map[string]any{"enabled": true, "httpsProxy": httpsProxy, "httpProxy": httpsProxy, "noProxy": wantNoProxy}
	if got := at(t, tree, keycloakProxy); !reflect.DeepEqual(got, wantKeycloak) {
		t.Errorf("keycloak.proxy = %#v, want %#v", got, wantKeycloak)
	}
	wantOpts := map[string]any{"https_proxy": httpsProxy, "http_proxy": httpsProxy, "no_proxy": wantNoProxy}
	if got := at(t, tree, apisixProxyOpts); !reflect.DeepEqual(got, wantOpts) {
		t.Errorf("APISIX proxy_opts = %#v, want %#v", got, wantOpts)
	}
}

// TestProxyGate checks that proxy.https switches the group: with the reserved refs seeded but no HTTPS proxy, no
// component gets a key and neither proxy.http nor proxy.no derives.
func TestProxyGate(t *testing.T) {
	tests := []struct {
		name    string
		answers map[string]any
	}{
		{name: "nothing given", answers: map[string]any{}},
		{name: "proxy.no given", answers: map[string]any{proxyNoID: "localhost"}},
		{name: "proxy.http given", answers: map[string]any{proxyHTTPID: httpsProxy}},
	}
	b := providerBundle(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, l := range renderShipped(t, tc.answers, oneRoute).Leaves {
				if slices.Contains(proxyIDs, l.Provenance.Foundation) {
					t.Errorf("%s renders from %s", l.Path, l.Provenance.Foundation)
				}
			}
			p := profile.Profile{Answers: tc.answers, Target: profile.Target{ReleaseName: release, Namespace: namespace}}
			answers, err := Answers(b, p)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{proxyHTTPID, proxyNoID} {
				if _, got := answers[id]; got != (tc.answers[id] != nil) {
					t.Errorf("Answers has %s: %v, given: %v", id, got, tc.answers[id] != nil)
				}
			}
		})
	}
}

// TestProxyNeedsNoProxy checks that no proxy goes out without NO_PROXY: while the proxy.no default cannot derive
// because Stage 1 leaves the release name or namespace unanswered, nothing in the group renders until proxy.no is given.
func TestProxyNeedsNoProxy(t *testing.T) {
	tests := []struct {
		name    string
		target  profile.Target
		answers map[string]any
		renders bool
	}{
		{name: "release unanswered", target: profile.Target{Namespace: namespace},
			answers: map[string]any{proxyHTTPSID: httpsProxy}},
		{name: "namespace unanswered", target: profile.Target{ReleaseName: release},
			answers: map[string]any{proxyHTTPSID: httpsProxy}},
		{name: "release unanswered, proxy.no given", target: profile.Target{Namespace: namespace},
			answers: map[string]any{proxyHTTPSID: httpsProxy, proxyNoID: noProxyForms}, renders: true},
		{name: "namespace unanswered, proxy.no given", target: profile.Target{ReleaseName: release},
			answers: map[string]any{proxyHTTPSID: httpsProxy, proxyNoID: noProxyForms}, renders: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reached := map[string]map[string]bool{}
			for _, l := range renderShippedFor(t, tc.answers, oneRoute, tc.target).Leaves {
				if id := l.Provenance.Foundation; slices.Contains(proxyIDs, id) {
					if reached[id] == nil {
						reached[id] = map[string]bool{}
					}
					reached[id][l.Path[:2].String()] = true
				}
			}
			for _, id := range proxyIDs {
				got := slices.Sorted(maps.Keys(reached[id]))
				if tc.renders && !reflect.DeepEqual(got, proxyComponents) {
					t.Errorf("%s reaches %v, want %v", id, got, proxyComponents)
				}
				if !tc.renders && len(got) > 0 {
					t.Errorf("%s renders into %v without proxy.no", id, got)
				}
			}
		})
	}
}
