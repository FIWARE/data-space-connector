package model

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"
)

const (
	validFoundations   = "testdata/foundations/valid.yaml"
	invalidFoundations = "testdata/foundations/invalid.yaml"
	foundationsHead    = "chart: " + chart + "\nversion: 1.0.0\nrevision: 1\ntopLevelKeys: [keycloak, scorpio]\n"
)

func loadFixture(t *testing.T, file string) Bundle {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Versions(fstest.MapFS{fixtureFile: {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	return got[0]
}

func TestFoundationsCompile(t *testing.T) {
	b := loadFixture(t, validFoundations)
	if got, want := b.FoundationOrder(), []string{"domain.base", "host.keycloak", "org.displayName"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FoundationOrder = %v, want %v", got, want)
	}
	if _, ok := b.Foundation("host.verifier"); ok {
		t.Error("Foundation(host.verifier) found an undeclared id")
	}
	host, ok := b.Foundation("host.keycloak")
	if !ok {
		t.Fatal("Foundation(host.keycloak) not found")
	}
	if want := []string{"domain.base"}; !reflect.DeepEqual(host.Refs, want) || host.DefaultTemplate() == nil {
		t.Errorf("host.keycloak refs = %v (default %v), want %v with a default", host.Refs, host.DefaultTemplate(), want)
	}
	targets := []struct {
		path  string
		fn    Fn
		refs  []string
		value any
		tpls  int
	}{
		{"keycloak.ingress.hostname", FnCopy, []string{"host.keycloak"}, nil, 1},
		{"keycloak.ingress.tls[0].hosts[0]", FnTpl, []string{"host.keycloak"}, nil, 1},
		{"keycloak.ingress.tls[0].secretName", FnTpl, []string{"host.keycloak"}, nil, 1},
		{`keycloak.ingress.annotations."cert-manager.io/alt-names"`, FnJoin, []string{"domain.base", "host.keycloak"}, nil, 2},
	}
	for i, want := range targets {
		got := host.Targets[i]
		if got.Path.String() != want.path || got.Fn != want.fn || !reflect.DeepEqual(got.Refs, want.refs) ||
			got.Value != want.value || len(got.Templates()) != want.tpls {
			t.Errorf("targets[%d] = %s %s %v %v (%d templates), want %+v", i, got.Path, got.Fn, got.Refs, got.Value, len(got.Templates()), want)
		}
	}
	base, _ := b.Foundation("domain.base")
	if c := base.Targets[1]; c.Fn != FnConst || c.Value != "/" || len(c.Refs) != 0 || c.Templates() != nil {
		t.Errorf("const target = %+v, want the scalar / without refs or templates", c)
	}
}

func TestFoundationsValidation(t *testing.T) {
	data, err := os.ReadFile(invalidFoundations)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string    `yaml:"name"`
		Err         string    `yaml:"err"`
		Foundations yaml.Node `yaml:"foundations"`
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			body, err := yaml.Marshal(map[string]*yaml.Node{"foundations": &tc.Foundations})
			if err != nil {
				t.Fatal(err)
			}
			_, err = Versions(fstest.MapFS{fixtureFile: {Data: []byte(foundationsHead + string(body))}})
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.Err) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.Err)
			}
		})
	}
}

func TestFoundationTypeCheck(t *testing.T) {
	tests := []struct {
		typ   FoundationType
		value any
		ok    bool
	}{
		{TypeString, "ACME Corp", true},
		{TypeString, " ACME", false},
		{TypeString, "", false},
		{TypeString, "two\nlines", false},
		{TypeString, "R&D Labs", true},
		{TypeString, `ACME "Labs"`, false},
		{TypeString, `R&D\Ops`, false},
		{TypeString, "<b>ACME</b>", false},
		{TypeString, "tab\there", false},
		{TypeString, 42, false},
		{TypeHostname, "localhost", true},
		{TypeHostname, "keycloak.127.0.0.1.nip.io", true},
		{TypeHostname, "Keycloak.example.org", false},
		{TypeHostname, "-a.example.org", false},
		{TypeHostname, "a..b", false},
		{TypeHostname, strings.Repeat("a.", maxHostnameLength/2) + "ab", false},
		{TypeDomain, "example.org", true},
		{TypeDomain, "localhost", false},
		{TypeDomain, "https://example.org", false},
		{FoundationType("number"), "1", false},
	}
	for _, tc := range tests {
		if err := tc.typ.Check(tc.value); (err == nil) != tc.ok {
			t.Errorf("%s.Check(%q) = %v, want ok %v", tc.typ, tc.value, err, tc.ok)
		}
	}
}
