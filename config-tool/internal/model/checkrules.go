package model

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// Severity says whether an L2 finding blocks generate (ADR 0021).
type Severity string

// Severities, most severe first.
const (
	// SeverityError blocks generate.
	SeverityError Severity = "error"
	// SeverityWarning is shown but never blocks.
	SeverityWarning Severity = "warning"
)

// Field ids a check rule may address; they are the open-field ids the UI links to.
const (
	// FieldTargetEnvironment is the Target stage's environment.
	FieldTargetEnvironment = "target.environment"
	// AnswerFieldPrefix starts the field id of a foundation answer: answers.<foundation id>.
	AnswerFieldPrefix = "answers."
)

// docAnchor separates a doc path from its anchor.
const docAnchor = "#"

var (
	knownSeverities = []Severity{SeverityError, SeverityWarning}
	// targetFields are the Target fields a check rule may address, with the values each takes. They mirror
	// profile.Target and profile.Environments, which import this package; internal/check's test keeps them equal.
	targetFields = map[string]struct {
		stage  Stage
		values []string
	}{
		FieldTargetEnvironment: {StageTarget, []string{"development", "production"}},
	}
)

// Rank orders severities most severe first.
func (s Severity) Rank() int {
	return slices.Index(knownSeverities, s)
}

// Rank orders stages in wizard order.
func (s Stage) Rank() int {
	return slices.Index(knownStages, s)
}

// CheckRule is an L2 rule as written in a bundle's rules list: a finding on Field while every When holds.
type CheckRule struct {
	ID       string     `yaml:"id"`
	Severity Severity   `yaml:"severity"`
	Stage    Stage      `yaml:"stage"`
	Field    string     `yaml:"field"`
	When     []RuleWhen `yaml:"when"`
	Message  string     `yaml:"message"`
	// Doc is a repo-relative documentation path, optionally with an anchor.
	Doc string `yaml:"doc"`
}

// RuleWhen holds while the resolved Field equals Equals; an unanswered field never matches.
type RuleWhen struct {
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
}

// CheckRules returns the validated check rules in bundle order.
func (b Bundle) CheckRules() []CheckRule {
	return slices.Clone(b.Checks)
}

// TargetFieldIDs lists the Target fields a check rule may address, sorted.
func TargetFieldIDs() []string {
	return slices.Sorted(maps.Keys(targetFields))
}

// TargetFieldValues lists the values a rule may compare Target field id with; nil for an unknown id.
func TargetFieldValues(id string) []string {
	return slices.Clone(targetFields[id].values)
}

// checkCheckRules validates rules against the compiled foundations fs.
func checkCheckRules(rules []CheckRule, fs []CompiledFoundation) error {
	seen := map[string]int{}
	for i, r := range rules {
		if j, dup := seen[r.ID]; dup {
			return fmt.Errorf("rules[%d]: id %q is already used by rules[%d]", i, r.ID, j)
		}
		seen[r.ID] = i
		if err := checkCheckRule(r, fs); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	return nil
}

func checkCheckRule(r CheckRule, fs []CompiledFoundation) error {
	switch {
	case r.ID == "":
		return errors.New("id is missing")
	case !slices.Contains(knownSeverities, r.Severity):
		return fmt.Errorf("severity %q is unknown", r.Severity)
	case !slices.Contains(knownStages, r.Stage):
		return fmt.Errorf("stage %q is unknown", r.Stage)
	case r.Message == "":
		return errors.New("message is missing")
	case len(r.When) == 0:
		return errors.New("when is missing")
	}
	stage, _, err := addressable(r.Field, fs)
	if err != nil {
		return fmt.Errorf("field: %w", err)
	}
	// The stage labels and links the finding, so it has to be the stage that shows the field.
	if stage != r.Stage {
		return fmt.Errorf("stage %s is not %s, the stage of field %s", r.Stage, stage, r.Field)
	}
	for i, w := range r.When {
		_, values, err := addressable(w.Field, fs)
		if err != nil {
			return fmt.Errorf("when[%d]: %w", i, err)
		}
		if values != nil && !slices.Contains(values, w.Equals) {
			return fmt.Errorf("when[%d]: %q is not a value of %s (%s)", i, w.Equals, w.Field, strings.Join(values, ", "))
		}
	}
	return checkDoc(r.Doc)
}

// addressable returns the stage of field and, when it takes a fixed set of values, those values.
func addressable(field string, fs []CompiledFoundation) (Stage, []string, error) {
	if t, ok := targetFields[field]; ok {
		return t.stage, t.values, nil
	}
	id, ok := strings.CutPrefix(field, AnswerFieldPrefix)
	i := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == id })
	if !ok || i < 0 {
		return "", nil, fmt.Errorf("%q is neither one of %s nor %s<foundation id>", field, strings.Join(TargetFieldIDs(), ", "), AnswerFieldPrefix)
	}
	if fs[i].Type == TypeChoice {
		return fs[i].Stage, fs[i].optionValues(), nil
	}
	return fs[i].Stage, nil, nil
}

// checkDoc refuses a doc that is not a clean repo-relative path, such as a URL or one leaving the repo.
func checkDoc(doc string) error {
	p, _, _ := strings.Cut(doc, docAnchor)
	if p == "" || path.IsAbs(p) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, ":") {
		return fmt.Errorf("doc %q is not a repo-relative path", doc)
	}
	return nil
}
