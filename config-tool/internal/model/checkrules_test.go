package model

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	invalidRules = "testdata/rules/invalid.yaml"
	ruleHead     = didFoundations
)

func TestCheckRulesValidation(t *testing.T) {
	head, err := os.ReadFile(ruleHead)
	if err != nil {
		t.Fatal(err)
	}
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
	head, err := os.ReadFile(ruleHead)
	if err != nil {
		t.Fatal(err)
	}
	const rules = `rules:
  - id: r
    severity: error
    stage: foundations
    field: answers.domain.base
    when: [{field: target.environment, equals: production}, {field: answers.did.method, equals: "did:key"}]
    message: M
    doc: doc/a.md#b
`
	got, err := Versions(fstest.MapFS{fixtureFile: {Data: append(head, rules...)}})
	if err != nil {
		t.Fatal(err)
	}
	want := []CheckRule{{
		ID: "r", Severity: SeverityError, Stage: StageFoundations, Field: "answers.domain.base",
		When:    []RuleWhen{{Field: FieldTargetEnvironment, Equals: "production"}, {Field: "answers.did.method", Equals: "did:key"}},
		Message: "M", Doc: "doc/a.md#b",
	}}
	if rs := got[0].CheckRules(); !reflect.DeepEqual(rs, want) {
		t.Errorf("CheckRules() = %+v, want %+v", rs, want)
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
