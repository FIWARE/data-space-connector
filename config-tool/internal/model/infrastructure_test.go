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
)

const (
	validInfrastructure   = "testdata/infrastructure/valid.yaml"
	invalidInfrastructure = "testdata/infrastructure/invalid.yaml"
	modeAnswer            = "infra.certManager.mode"
	issuerAnnotation      = `annotations."cert-manager.io/cluster-issuer"`
)

func TestInfrastructureCompile(t *testing.T) {
	b := loadFixture(t, validInfrastructure)
	declared := make([]string, len(b.Foundations))
	for i, f := range b.Foundations {
		declared[i] = f.ID
	}
	if want := []string{"domain.base", "host.keycloak"}; !reflect.DeepEqual(declared, want) {
		t.Errorf("declared foundations = %v, want %v: the Stage 3 list must not change", declared, want)
	}
	var compiled []string
	for _, f := range b.CompiledFoundations() {
		compiled = append(compiled, f.ID)
	}
	want := []string{
		"domain.base", "host.keycloak",
		modeAnswer, "infra.certManager.type", "infra.certManager.acmeEmail", "infra.certManager.namespace",
		"infra.certManager.issuer", "infra.certManager.secret.apisix", "infra.certManager.secret.keycloak",
		"infra.database.mode",
	}
	if !reflect.DeepEqual(compiled, want) {
		t.Errorf("CompiledFoundations = %v, want %v", compiled, want)
	}
	if order := b.FoundationOrder(); len(order) != len(want) ||
		slices.Index(order, "infra.certManager.secret.keycloak") < slices.Index(order, "host.keycloak") {
		t.Errorf("FoundationOrder = %v, want every compiled id, each after its refs", order)
	}
	mode, ok := b.Foundation(modeAnswer)
	if !ok || mode.Type != TypeChoice || mode.Stage != StageInfrastructure || mode.DefaultTemplate() != nil {
		t.Fatalf("Foundation(%s) = %+v, %v; want a choice of stage infrastructure without a default", modeAnswer, mode, ok)
	}
	if got := mode.optionValues(); !reflect.DeepEqual(got, knownModes) {
		t.Errorf("mode options = %v, want %v", got, knownModes)
	}
	dep, ok := b.Dependency("certManager")
	if !ok || len(b.Dependencies()) != 2 {
		t.Fatalf("Dependency(certManager) = %v, Dependencies = %d; want it and two", ok, len(b.Dependencies()))
	}
	if cluster, ok := dep.Mode(ModeCluster); !ok || len(cluster.Questions) != 1 || cluster.Questions[0].ID != "issuer" {
		t.Errorf("Mode(cluster) = %+v, %v; want the one question issuer", cluster, ok)
	}
	if _, ok := dep.Mode("hosted"); ok {
		t.Error("Mode(hosted) found a mode the dependency does not offer")
	}
}

// TestInfrastructureTargets pins the When gating, the per-ingress requires and the replacing Secret names.
func TestInfrastructureTargets(t *testing.T) {
	b := loadFixture(t, validInfrastructure)
	modeIs := func(m string) TargetCondition { return TargetCondition{ID: modeAnswer, Equals: m} }
	tests := []struct {
		id       string
		path     string
		fn       Fn
		when     []TargetCondition
		requires []string
		replaces bool
	}{
		{modeAnswer, "cert-manager.enabled", FnConst, []TargetCondition{modeIs(ModeBundled)}, nil, false},
		{modeAnswer, "apisix.ingress." + issuerAnnotation, FnTpl, []TargetCondition{modeIs(ModeBundled)}, []string{"domain.base"}, false},
		{modeAnswer, "keycloak.ingress." + issuerAnnotation, FnCopy, []TargetCondition{modeIs(ModeCluster)}, []string{"host.keycloak"}, false},
		{"infra.certManager.type", "certManagerResources.type", FnCopy, []TargetCondition{modeIs(ModeBundled)}, nil, false},
		{"infra.certManager.acmeEmail", "certManagerResources.acmeEmail", FnCopy,
			[]TargetCondition{modeIs(ModeBundled), {ID: "infra.certManager.type", Equals: "prod"}}, nil, false},
		{"infra.certManager.secret.apisix", "apisix.ingress.tls[0].secretName", FnCopy, []TargetCondition{modeIs(ModeExternal)}, []string{"domain.base"}, true},
		{"infra.certManager.secret.keycloak", "keycloak.ingress.tls[0].secretName", FnCopy, []TargetCondition{modeIs(ModeExternal)}, []string{"host.keycloak"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.id+" "+tc.path+" "+string(tc.fn), func(t *testing.T) {
			f, _ := b.Foundation(tc.id)
			i := slices.IndexFunc(f.Targets, func(ct CompiledTarget) bool {
				return ct.Path.String() == tc.path && ct.Fn == tc.fn && reflect.DeepEqual(ct.When, tc.when)
			})
			if i < 0 {
				t.Fatalf("no %s target at %s with when %v among %+v", tc.fn, tc.path, tc.when, f.Targets)
			}
			if got := f.Targets[i]; !reflect.DeepEqual(got.Requires, tc.requires) || got.Replaces != tc.replaces {
				t.Errorf("requires %v, replaces %v; want %v, %v", got.Requires, got.Replaces, tc.requires, tc.replaces)
			}
		})
	}
}

// TestInfrastructureReplacedDefaults checks that a replacing question derives today's Secret name.
func TestInfrastructureReplacedDefaults(t *testing.T) {
	b := loadFixture(t, validInfrastructure)
	answers := map[string]any{"domain": map[string]any{"base": "example.org"}, "host": map[string]any{"keycloak": "kc.example.org"}}
	tests := []struct {
		id   string
		refs []string
		want string
	}{
		{"infra.certManager.secret.apisix", nil, "apisix-provider-tls"},
		{"infra.certManager.secret.keycloak", []string{"host.keycloak"}, "kc.example.org-tls"},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			f, _ := b.Foundation(tc.id)
			if f.DefaultTemplate() == nil || !reflect.DeepEqual(f.DefaultRefs(), tc.refs) {
				t.Fatalf("default %v with refs %v, want one with refs %v", f.DefaultTemplate(), f.DefaultRefs(), tc.refs)
			}
			if got, err := Execute(f.DefaultTemplate(), answers); err != nil || got != tc.want {
				t.Errorf("default = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestInfrastructureValidation(t *testing.T) {
	valid, err := os.ReadFile(validInfrastructure)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(invalidInfrastructure)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string               `yaml:"name"`
		Err      string               `yaml:"err"`
		Override map[string]yaml.Node `yaml:"override"`
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var doc yaml.Node
			if err := yaml.Unmarshal(valid, &doc); err != nil {
				t.Fatal(err)
			}
			root := doc.Content[0]
			for k, v := range tc.Override {
				i := slices.IndexFunc(root.Content, func(n *yaml.Node) bool { return n.Value == k })
				if i < 0 || i%2 != 0 {
					t.Fatalf("override %q is no top-level key of %s", k, validInfrastructure)
				}
				root.Content[i+1] = &v
			}
			body, err := yaml.Marshal(&doc)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Versions(fstest.MapFS{fixtureFile: {Data: body}})
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.Err) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.Err)
			}
		})
	}
}

func TestInfraFieldAnswer(t *testing.T) {
	b := loadFixture(t, validInfrastructure)
	tests := []struct {
		field string
		id    string
		ok    bool
	}{
		{"infrastructure.certManager", modeAnswer, true},
		{"infrastructure.certManager.issuer", "infra.certManager.issuer", true},
		{"infrastructure.certManager.secret.keycloak", "infra.certManager.secret.keycloak", true},
		{"infrastructure.database", "infra.database.mode", true},
		{"infrastructure.certManager.mode", "", false},
		{"infrastructure.certManager.secret", "", false},
		{"infrastructure.certManager.nope", "", false},
		{"infrastructure.nope", "", false},
		{"infrastructure.", "", false},
		{"infrastructure.certManager.", "", false},
		{"answers.infra.certManager.issuer", "", false},
		{"answers.domain.base", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			id, ok := b.InfraFieldAnswer(tc.field)
			if id != tc.id || ok != tc.ok {
				t.Fatalf("InfraFieldAnswer = %q, %v; want %q, %v", id, ok, tc.id, tc.ok)
			}
			if !ok {
				return
			}
			if field, ok := InfraAnswerField(id); !ok || field != tc.field {
				t.Errorf("InfraAnswerField(%s) = %q, %v; want %q", id, field, ok, tc.field)
			}
		})
	}
	for _, id := range []string{"domain.base", "infra.certManager", "infra.", "infrastructure.certManager"} {
		if field, ok := InfraAnswerField(id); ok {
			t.Errorf("InfraAnswerField(%s) = %q, want false", id, field)
		}
	}
}

// TestConstTargetGating refuses a top-level const unless a list index, requires or a mode condition gates it.
func TestConstTargetGating(t *testing.T) {
	const path = "keycloak.enabled"
	when := []TargetCondition{{ID: modeAnswer, Equals: ModeBundled}}
	tests := []struct {
		name     string
		path     string
		requires []string
		when     []TargetCondition
		ok       bool
	}{
		{"ungated", path, nil, nil, false},
		{"mode condition", path, nil, when, true},
		{"requires", path, []string{"domain.base"}, nil, true},
		{"list index", "keycloak.hosts[0]", nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := compileTarget("a", Target{Path: tc.path, Fn: FnConst, Args: []any{true}, Requires: tc.requires}, tc.when, []string{"keycloak"})
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok %v", err, tc.ok)
			}
			if err == nil && !reflect.DeepEqual(c.When, tc.when) {
				t.Errorf("When = %v, want %v", c.When, tc.when)
			}
		})
	}
}

// TestCheckTargetPathsWhen accepts a shared leaf only for exclusive targets or a replacing one over an
// unconditional one.
func TestCheckTargetPathsWhen(t *testing.T) {
	const path = "keycloak.ingress.tls[0].secretName"
	p, err := roundtrip.ParsePath(path)
	if err != nil {
		t.Fatal(err)
	}
	target := func(replaces bool, when ...TargetCondition) CompiledTarget {
		return CompiledTarget{Path: p, Fn: FnConst, When: when, Replaces: replaces}
	}
	mode := func(m string) TargetCondition { return TargetCondition{ID: modeAnswer, Equals: m} }
	plain := CompiledFoundation{ID: "host.keycloak", Targets: []CompiledTarget{target(false)}}
	tests := []struct {
		name string
		fs   []CompiledFoundation
		err  string
	}{
		{"distinct modes", []CompiledFoundation{
			{ID: "x", Targets: []CompiledTarget{target(false, mode(ModeBundled))}},
			{ID: "y", Targets: []CompiledTarget{target(false, mode(ModeCluster))}},
		}, ""},
		{"one mode", []CompiledFoundation{
			{ID: "x", Targets: []CompiledTarget{target(false, mode(ModeBundled))}},
			{ID: "y", Targets: []CompiledTarget{target(false, mode(ModeBundled), TargetCondition{ID: "q", Equals: "a"})}},
		}, "already written by foundations[0] targets[0]"},
		{"conditions on different ids", []CompiledFoundation{
			{ID: "x", Targets: []CompiledTarget{target(false, mode(ModeBundled))}},
			{ID: "y", Targets: []CompiledTarget{target(false, TargetCondition{ID: "q", Equals: "a"})}},
		}, "already written"},
		{"Only against When on the same id", []CompiledFoundation{
			{ID: modeAnswer, Targets: []CompiledTarget{{Path: p, Fn: FnConst, Only: ModeCluster}}},
			{ID: "y", Targets: []CompiledTarget{target(false, mode(ModeBundled))}},
		}, ""},
		{"conditioned beside unconditional", []CompiledFoundation{plain, {ID: "y", Targets: []CompiledTarget{target(false, mode(ModeExternal))}}},
			"already written"},
		{"replacing an unconditional target", []CompiledFoundation{plain, {ID: "y", Targets: []CompiledTarget{target(true, mode(ModeExternal))}}}, ""},
		{"replacing first", []CompiledFoundation{{ID: "y", Targets: []CompiledTarget{target(true, mode(ModeExternal))}}, plain}, ""},
		{"replacing nothing", []CompiledFoundation{{ID: "y", Targets: []CompiledTarget{target(true, mode(ModeExternal))}}},
			"replaces no unconditional foundation target"},
		{"replacing a conditioned target", []CompiledFoundation{
			{ID: "x", Targets: []CompiledTarget{target(false, mode(ModeBundled))}},
			{ID: "y", Targets: []CompiledTarget{target(true, mode(ModeExternal))}},
		}, "replaces no unconditional foundation target"},
		{"two replacing in one mode", []CompiledFoundation{plain,
			{ID: "y", Targets: []CompiledTarget{target(true, mode(ModeExternal))}},
			{ID: "z", Targets: []CompiledTarget{target(true, mode(ModeExternal))}},
		}, "already written by foundations[1] targets[0]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkTargetPaths(tc.fs)
			switch {
			case tc.err == "" && err != nil:
				t.Fatalf("err = %v, want none", err)
			case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
				t.Fatalf("err = %v, want it to contain %q", err, tc.err)
			}
		})
	}
}

func TestTargetConditionHolds(t *testing.T) {
	c := TargetCondition{ID: modeAnswer, Equals: ModeCluster}
	tests := []struct {
		name    string
		answers map[string]any
		want    bool
	}{
		{"equal", map[string]any{modeAnswer: ModeCluster}, true},
		{"other value", map[string]any{modeAnswer: ModeExternal}, false},
		{"unset", map[string]any{}, false},
		{"not a string", map[string]any{modeAnswer: true}, false},
	}
	for _, tc := range tests {
		if got := c.Holds(tc.answers); got != tc.want {
			t.Errorf("%s: Holds = %v, want %v", tc.name, got, tc.want)
		}
	}
}
