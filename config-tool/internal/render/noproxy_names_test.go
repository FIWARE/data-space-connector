package render

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	// statusListServiceTemplate writes the status-list-server Service name literally.
	statusListServiceTemplate = "../../../charts/data-space-connector/templates/status-list-server-service.yaml"
	otherRelease              = "other"
)

var metadataName = regexp.MustCompile(`(?m)^metadata:\n  name: (\S+)$`)

// TestNoProxyServiceNames pins every noProxyServices row to the chart: a fixed name to the umbrella override that
// fixes it or to the template literal, a release-prefixed name to overrides and a nameOverride nobody sets. A renamed
// service fails.
func TestNoProxyServiceNames(t *testing.T) {
	umbrella, fixtures := parseValues(t, umbrellaValues), subchartFixtures(t)
	literals := map[string]bool{}
	for _, f := range fixtures {
		for _, n := range f.Names {
			literals[n] = true
		}
	}
	src, err := os.ReadFile(statusListServiceTemplate)
	if err != nil {
		t.Fatal(err)
	}
	m := metadataName.FindSubmatch(src)
	if m == nil {
		t.Fatalf("%s: no metadata.name", statusListServiceTemplate)
	}
	literals[string(m[1])] = true
	for i, s := range providerBundle(t).NoProxyServices() {
		name, err := s.Render(release, namespace)
		if err != nil {
			t.Fatal(err)
		}
		other, err := s.Render(otherRelease, namespace)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case name != other:
			if len(s.Overrides) == 0 || s.NameOverride == nil {
				t.Errorf("noProxyServices[%d] %s: a release-prefixed name needs the overrides and nameOverride that rename it", i, name)
			}
			for _, o := range append(slices.Clone(s.Overrides), s.NameOverride) {
				if o == nil {
					continue
				}
				if n, err := umbrella.Lookup(o); err == nil && n.Value != "" {
					t.Errorf("noProxyServices[%d] %s: the umbrella sets %s to %q; the name is fixed now", i, name, o, n.Value)
				}
				root, _ := o[0].Key()
				v, ok := lookupValue(fixtures[root].Values, o[1:])
				if !ok {
					t.Errorf("noProxyServices[%d] %s: no subchart fixture declares %s", i, name, o)
				} else if v != nil && v != "" {
					t.Errorf("noProxyServices[%d] %s: the subchart defaults %s to %#v", i, name, o, v)
				}
			}
		case len(s.Overrides) == 0:
			if !literals[name] {
				t.Errorf("noProxyServices[%d] %s: no chart template writes the name literally", i, name)
			}
		default:
			n, err := umbrella.Lookup(s.Overrides[0])
			if err != nil || n.Value != name {
				t.Errorf("noProxyServices[%d] %s: the umbrella's %s does not fix the name: %v", i, name, s.Overrides[0], err)
			}
		}
	}
}

// TestNoProxyNameOverride checks that a nameOverride replaces the chart name as Helm's fullname does.
func TestNoProxyNameOverride(t *testing.T) {
	var keycloak model.CompiledNoProxyService
	for _, s := range providerBundle(t).NoProxyServices() {
		if s.NameOverride.String() == "keycloak.nameOverride" {
			keycloak = s
		}
	}
	if keycloak.Name == nil {
		t.Fatal("no noProxyServices row with nameOverride keycloak.nameOverride")
	}
	for _, tc := range []struct {
		name, release, nameOverride, want string
	}{
		{"unset keeps the chart name", release, "", release + "-keycloak"},
		{"set replaces the chart name", release, "idp", release + "-idp"},
		{"release containing the override", "my-idp", "idp", "my-idp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := keycloak.RenderNamed(tc.release, namespace, tc.nameOverride)
			if err != nil || got != tc.want {
				t.Errorf("RenderNamed = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// TestNoProxyDefaultNames checks that the proxy.no default lists exactly the table's names, in table order.
func TestNoProxyDefaultNames(t *testing.T) {
	b := providerBundle(t)
	names := []string{noProxyForms}
	for _, s := range b.NoProxyServices() {
		name, err := s.Render(release, namespace)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	p := profile.Profile{Answers: map[string]any{proxyHTTPSID: httpsProxy}, Target: profile.Target{ReleaseName: release, Namespace: namespace}}
	answers, err := Answers(b, p)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := answers[proxyNoID], strings.Join(names, ","); !reflect.DeepEqual(got, want) {
		t.Errorf("proxy.no default = %q, want %q", got, want)
	}
}
