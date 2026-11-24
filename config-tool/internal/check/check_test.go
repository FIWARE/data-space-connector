package check

import (
	"os"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
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

	tirRule          = "tir-address-for-verification"
	tirAnswer        = "dataspace.tirAddress"
	tirAddress       = "https://tir.example.org/"
	verification     = "access.verification"
	verificationOn   = "testdata/verification-on.dscprofile.yaml"
	tirAddressValues = "testdata/tir-address.values.yaml"
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

// readProfile decodes a profile fixture.
func readProfile(t *testing.T, file string) profile.Profile {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	p, err := profile.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// loadedAnswers loads file against the shipped bundles and returns the derived answers SeedAnswers adds to answers.
func loadedAnswers(t *testing.T, file string, answers map[string]any) map[string]any {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	all, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	res, err := load.Load(src, file, all, load.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return load.SeedAnswers(res.Report, answers)
}

func TestTIRAddressForVerification(t *testing.T) {
	b := shipped(t)
	type loading int
	const (
		notLoaded loading = iota
		loadedOnly
		loadedAndSeeded
	)
	tests := []struct {
		name string
		// features replaces the fixture's feature states when non-nil.
		features map[string]profile.FeatureState
		givenTIR bool
		load     loading
		fires    bool
	}{
		{name: "verification on, TIR unset", fires: true},
		{name: "verification on, TIR set", givenTIR: true},
		{name: "verification off, TIR unset", features: map[string]profile.FeatureState{
			verification: {Enabled: false, Source: profile.SourceUser}}},
		{name: "verification absent, TIR unset", features: map[string]profile.FeatureState{}},
		{name: "loaded TIR, seeded", load: loadedAndSeeded},
		// Nothing but the seeding carries the loaded address; Evaluate reads nothing from the load result.
		{name: "loaded TIR, not seeded", load: loadedOnly, fires: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := readProfile(t, verificationOn)
			if tc.features != nil {
				p.Features = tc.features
			}
			if tc.givenTIR {
				p.Answers[tirAnswer] = tirAddress
			}
			if tc.load != notLoaded {
				seeded := loadedAnswers(t, tirAddressValues, p.Answers)
				if tc.load == loadedAndSeeded {
					p.Answers = seeded
				}
			}
			got, err := Evaluate(b, p)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.fires {
				if len(got) != 0 {
					t.Errorf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want exactly %s", got, tirRule)
			}
			f := got[0]
			if f.Rule != tirRule || f.Severity != model.SeverityError || f.Stage != model.StageFoundations ||
				f.Field != model.AnswerFieldPrefix+tirAnswer || f.Message == "" || f.Doc == "" {
				t.Errorf("finding = %+v, want a %s error on answers.%s in stage foundations", f, tirRule, tirAnswer)
			}
			if !Blocking(got) {
				t.Errorf("Blocking(%+v) = false, want the error to block", got)
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
	envField := profile.TargetFieldPrefix + profile.FieldEnvironment
	if got, want := model.TargetFieldIDs(), []string{envField}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetFieldIDs() = %v, want %v; teach value to read a new field", got, want)
	}
	want := make([]string, len(profile.Environments))
	for i, env := range profile.Environments {
		want[i] = string(env)
	}
	if got := model.TargetFieldValues(envField); !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetFieldValues(%q) = %v, want profile.Environments %v", envField, got, want)
	}
	for _, env := range profile.Environments {
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
