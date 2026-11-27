package model

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	elementFoundations        = "testdata/foundations/elements.yaml"
	invalidElementFoundations = "testdata/foundations/invalid-elements.yaml"
	elementsHead              = "chart: " + chart + "\nversion: 1.0.0\nrevision: 1\ntopLevelKeys: [keycloak]\n"
)

func TestElementsCompile(t *testing.T) {
	b := loadFixture(t, elementFoundations)
	if got := len(b.ListTable()); got != 2 {
		t.Fatalf("ListTable has %d rows, want 2", got)
	}
	l, ok := b.List(roundtrip.Path{roundtrip.Key("keycloak"), roundtrip.Key("extraVolumeMounts")})
	if !ok || l.Key != "name" || !reflect.DeepEqual(l.Unique, []string{"mountPath"}) {
		t.Errorf("List(keycloak.extraVolumeMounts) = %+v, %v", l, ok)
	}
	ca, _ := b.Foundation("ca.secret")
	env := ca.Targets[0]
	if env.Element == nil || env.Element.KeyField != "name" || env.Mechanism != "keycloak-ca" || !reflect.DeepEqual(env.Requires, []string{"ca.secret"}) {
		t.Errorf("targets[0] = %+v", env)
	}
	exec := func(tpl *template.Template) (string, error) {
		var s strings.Builder
		err := tpl.Execute(&s, map[string]any{"ca": map[string]any{"secret": "my-ca"}})
		return s.String(), err
	}
	got, err := ca.Targets[2].Element.Expand(exec)
	want := map[string]any{"name": "CA_SECRET", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "my-ca", "key": "ca.crt"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Expand = %v, %v; want %v", got, err, want)
	}
	if !reflect.DeepEqual(ca.Targets[2].Refs, []string{"ca.secret"}) {
		t.Errorf("element refs = %v", ca.Targets[2].Refs)
	}
	if each := ca.Targets[3].Each.String(); each != "plugins.openid-connect.ssl_verify" {
		t.Errorf("each = %s", each)
	}
	no, _ := b.Foundation("proxy.no")
	if w := no.DefaultWhen; w == nil || !w.Set || w.ID != "proxy.https" {
		t.Errorf("proxy.no defaultWhen = %+v", w)
	}
	if got, want := no.DefaultRefs(), []string{"proxy.https", RefTargetNamespace, RefTargetRelease}; !reflect.DeepEqual(got, want) {
		t.Errorf("proxy.no DefaultRefs = %v, want %v", got, want)
	}
}

func TestElementsValidation(t *testing.T) {
	data, err := os.ReadFile(invalidElementFoundations)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string    `yaml:"name"`
		Err         string    `yaml:"err"`
		Lists       yaml.Node `yaml:"lists"`
		Foundations yaml.Node `yaml:"foundations"`
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			body, err := yaml.Marshal(map[string]*yaml.Node{"lists": &tc.Lists, "foundations": &tc.Foundations})
			if err != nil {
				t.Fatal(err)
			}
			_, err = Versions(fstest.MapFS{fixtureFile: {Data: []byte(elementsHead + string(body))}})
			if err == nil || !strings.Contains(err.Error(), tc.Err) {
				t.Errorf("err = %v, want it to contain %q", err, tc.Err)
			}
		})
	}
}

func TestConditionHolds(t *testing.T) {
	answers := map[string]any{"a": "x"}
	for _, tc := range []struct {
		name string
		c    Condition
		want bool
	}{
		{"equals holds", Condition{ID: "a", Equals: "x"}, true},
		{"equals differs", Condition{ID: "a", Equals: "y"}, false},
		{"equals unresolved", Condition{ID: "b", Equals: "x"}, false},
		{"set holds", Condition{ID: "a", Set: true}, true},
		{"set unresolved", Condition{ID: "b", Set: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.Holds(answers); got != tc.want {
				t.Errorf("Holds = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFullname(t *testing.T) {
	long := strings.Repeat("r", 54)
	for _, tc := range []struct {
		name, release, chart, want string
	}{
		{"release contains chart", "provider-keycloak", "keycloak", "provider-keycloak"},
		{"release equals chart", "keycloak", "keycloak", "keycloak"},
		{"prefixed", "provider", "keycloak", "provider-keycloak"},
		{"truncated to 63", long, "keycloak-x", (long + "-keycloak-x")[:maxResourceName]},
		{"trailing dash trimmed", strings.Repeat("r", 62), "keycloak", strings.Repeat("r", 62)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Fullname(tc.release, tc.chart); got != tc.want {
				t.Errorf("Fullname(%q, %q) = %q, want %q", tc.release, tc.chart, got, tc.want)
			}
		})
	}
}
