package check

import (
	"os"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	shippedChart   = "data-space-connector"
	shippedVersion = "10.16.0"
	fixtureDir     = "testdata"
	didKeyRule     = "did-key-in-production"
	didMethod      = "did.method"
	didKey         = "did:key"
	didWeb         = "did:web"
)

func shipped(t *testing.T) model.Bundle {
	t.Helper()
	all, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(b model.Bundle) bool { return b.Chart == shippedChart && b.Version == shippedVersion })
	if i < 0 {
		t.Fatalf("no bundle %s %s", shippedChart, shippedVersion)
	}
	return all[i]
}

func fixture(t *testing.T) model.Bundle {
	t.Helper()
	all, err := model.Versions(os.DirFS(fixtureDir))
	if err != nil {
		t.Fatal(err)
	}
	return all[0]
}

func profileOf(env profile.Environment, answers map[string]any) profile.Profile {
	return profile.Profile{Target: profile.Target{Environment: env}, Answers: answers}
}

func ids(findings []Finding) []string {
	out := []string{}
	for _, f := range findings {
		out = append(out, f.Rule)
	}
	return out
}

func TestDIDKeyInProduction(t *testing.T) {
	b := shipped(t)
	tests := []struct {
		name    string
		env     profile.Environment
		answers map[string]any
		want    bool
	}{
		{"development did:web", profile.EnvDevelopment, map[string]any{didMethod: didWeb}, false},
		{"development did:key", profile.EnvDevelopment, map[string]any{didMethod: didKey}, false},
		{"production did:web", profile.EnvProduction, map[string]any{didMethod: didWeb}, false},
		{"production did:key", profile.EnvProduction, map[string]any{didMethod: didKey}, true},
		{"production, method derived as did:web", profile.EnvProduction, nil, false},
		{"no environment did:key", "", map[string]any{didMethod: didKey}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(b, profileOf(tc.env, tc.answers))
			if err != nil {
				t.Fatal(err)
			}
			if !tc.want {
				if len(got) != 0 {
					t.Errorf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want exactly %s", got, didKeyRule)
			}
			f := got[0]
			if f.Rule != didKeyRule || f.Severity != model.SeverityWarning || f.Stage != model.StageFoundations ||
				f.Field != model.AnswerFieldPrefix+didMethod || f.Message == "" || f.Doc == "" {
				t.Errorf("finding = %+v, want a %s warning on answers.%s in stage foundations", f, didKeyRule, didMethod)
			}
			if Blocking(got) {
				t.Errorf("Blocking(%+v) = true, want a warning not to block", got)
			}
		})
	}
}

func TestEvaluateOrderAndBlocking(t *testing.T) {
	b := fixture(t)
	tests := []struct {
		name     string
		p        profile.Profile
		want     []string
		blocking bool
	}{
		{"none hold", profileOf(profile.EnvDevelopment, nil), []string{}, false},
		{"warnings only, target stage first", profileOf(profile.EnvProduction, nil), []string{"b-warning", "a-warning"}, false},
		{"one error", profileOf(profile.EnvDevelopment, map[string]any{didMethod: didKey}), []string{"c-error"}, true},
		{"errors before warnings, by stage then id", profileOf(profile.EnvProduction, map[string]any{didMethod: didKey}),
			[]string{"d-error", "c-error", "b-warning", "a-warning"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(b, tc.p)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids(got), tc.want) {
				t.Errorf("findings = %v, want %v", ids(got), tc.want)
			}
			if Blocking(got) != tc.blocking {
				t.Errorf("Blocking = %v, want %v", Blocking(got), tc.blocking)
			}
		})
	}
}

func TestEvaluateCopiesTheRule(t *testing.T) {
	got, err := Evaluate(fixture(t), profileOf(profile.EnvProduction, map[string]any{didMethod: didKey}))
	if err != nil {
		t.Fatal(err)
	}
	want := Finding{Rule: "d-error", Severity: model.SeverityError, Stage: model.StageTarget,
		Field: model.FieldTargetEnvironment, Message: "D", Doc: "doc/d.md#x"}
	if len(got) == 0 || got[0] != want {
		t.Errorf("first finding = %+v, want %+v", got, want)
	}
}

func TestEvaluateRefusesUnresolvedAnswers(t *testing.T) {
	if _, err := Evaluate(fixture(t), profileOf(profile.EnvProduction, map[string]any{"undeclared": "x"})); err == nil {
		t.Error("Evaluate with an undeclared answer = nil error, want one")
	}
}

// TestTargetFields keeps model's addressable Target fields in step with profile.Target, which model cannot import.
func TestTargetFields(t *testing.T) {
	envs := []profile.Environment{profile.EnvDevelopment, profile.EnvProduction}
	if got, want := model.TargetFieldIDs(), []string{profile.TargetFieldPrefix + profile.FieldEnvironment}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetFieldIDs() = %v, want %v; teach value to read a new field", got, want)
	}
	for _, env := range envs {
		t.Run(string(env), func(t *testing.T) {
			src := "chart: c\nversion: 1.0.0\nrevision: 1\nrules:\n" +
				"  - {id: r, severity: warning, stage: target, field: target.environment, when: [{field: target.environment, equals: " +
				string(env) + "}], message: M, doc: doc/a.md}\n"
			all, err := model.Versions(fstest.MapFS{"1.0.0/bundle.yaml": {Data: []byte(src)}})
			if err != nil {
				t.Fatalf("a rule on environment %s: %v", env, err)
			}
			got, err := Evaluate(all[0], profileOf(env, nil))
			if err != nil || len(got) != 1 {
				t.Errorf("Evaluate on environment %s = %+v, %v, want one finding", env, got, err)
			}
		})
	}
}
