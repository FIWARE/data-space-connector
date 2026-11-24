package model

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	invalidRules = "testdata/rules/invalid.yaml"
	ruleHead     = didFoundations
	// ruleFeatures gives the rule fixtures a catalogue for features.<id> to address.
	ruleFeatures = "features:\n  - {id: access.verification, title: T, summary: S, group: access}\n"
)

// readRuleHead returns the foundations and the catalogue the rule fixtures are written against.
func readRuleHead(t *testing.T) []byte {
	t.Helper()
	head, err := os.ReadFile(ruleHead)
	if err != nil {
		t.Fatal(err)
	}
	return append(head, ruleFeatures...)
}

func TestCheckRulesValidation(t *testing.T) {
	head := readRuleHead(t)
	data, err := os.ReadFile(invalidRules)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string    `yaml:"name"`
		Err   string    `yaml:"err"`
		Rules yaml.Node `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			body, err := yaml.Marshal(map[string]*yaml.Node{"rules": &tc.Rules})
			if err != nil {
				t.Fatal(err)
			}
			_, err = Versions(fstest.MapFS{fixtureFile: {Data: append(head, body...)}})
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.Err) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.Err)
			}
		})
	}
}

func TestCheckRules(t *testing.T) {
	head := readRuleHead(t)
	tests := []struct {
		name string
		when string
		want []RuleWhen
	}{
		{"target and choice answer", `[{field: target.environment, equals: production}, {field: answers.did.method, equals: "did:key"}]`,
			[]RuleWhen{{Field: FieldTargetEnvironment, Equals: "production"}, {Field: "answers.did.method", Equals: "did:key"}}},
		{"feature on", `[{field: features.access.verification, equals: "on"}]`,
			[]RuleWhen{{Field: FeatureFieldPrefix + "access.verification", Equals: FeatureOn}}},
		{"feature off", `[{field: features.access.verification, equals: "off"}]`,
			[]RuleWhen{{Field: FeatureFieldPrefix + "access.verification", Equals: FeatureOff}}},
		{"unset answer", `[{field: answers.domain.base, unset: true}]`,
			[]RuleWhen{{Field: "answers.domain.base", Unset: true}}},
		{"free-text answer equals", `[{field: answers.domain.base, equals: example.org}]`,
			[]RuleWhen{{Field: "answers.domain.base", Equals: "example.org"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rules := "rules:\n  - {id: r, severity: error, stage: foundations, field: answers.domain.base, when: " + tc.when +
				", message: M, doc: doc/a.md#b}\n"
			got, err := Versions(fstest.MapFS{fixtureFile: {Data: append(slices.Clone(head), rules...)}})
			if err != nil {
				t.Fatal(err)
			}
			want := []CheckRule{{
				ID: "r", Severity: SeverityError, Stage: StageFoundations, Field: "answers.domain.base",
				When: tc.want, Message: "M", Doc: "doc/a.md#b",
			}}
			if rs := got[0].CheckRules(); !reflect.DeepEqual(rs, want) {
				t.Errorf("CheckRules() = %+v, want %+v", rs, want)
			}
		})
	}
}

// TestEmbeddedCheckRuleDocs keeps every shipped rule's doc pointing at a file the repository has.
func TestEmbeddedCheckRuleDocs(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got {
		for _, r := range b.CheckRules() {
			file, _, _ := strings.Cut(r.Doc, docAnchor)
			if _, err := os.Stat(repoRoot + "/" + file); err != nil {
				t.Errorf("%s %s rule %s: doc %s: %v", b.Chart, b.Version, r.ID, r.Doc, err)
			}
		}
	}
}

func TestRank(t *testing.T) {
	if SeverityError.Rank() >= SeverityWarning.Rank() {
		t.Errorf("error ranks %d, warning %d; want error first", SeverityError.Rank(), SeverityWarning.Rank())
	}
	if StageTarget.Rank() >= StageFoundations.Rank() || StageFoundations.Rank() >= StageReview.Rank() {
		t.Errorf("stage ranks target %d, foundations %d, review %d; want wizard order",
			StageTarget.Rank(), StageFoundations.Rank(), StageReview.Rank())
	}
}
