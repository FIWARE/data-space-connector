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
	// FeatureFieldPrefix starts the field id of a catalogue feature, features.<feature id>; only a when addresses it.
	FeatureFieldPrefix = "features."
)

// Values a features.<id> field resolves to; a feature the profile does not record is off.
const (
	FeatureOn  = "on"
	FeatureOff = "off"
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

// RuleWhen holds while the resolved Field equals Equals, or with Unset while the answer Field is absent.
// It has exactly one of the two; an absent field never equals anything.
type RuleWhen struct {
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
	Unset  bool   `yaml:"unset"`
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

// checkCheckRules validates rules against the compiled foundations fs and the feature catalogue.
func checkCheckRules(rules []CheckRule, fs []CompiledFoundation, features []Feature) error {
	seen := map[string]int{}
	for i, r := range rules {
		if j, dup := seen[r.ID]; dup {
			return fmt.Errorf("rules[%d]: id %q is already used by rules[%d]", i, r.ID, j)
		}
		seen[r.ID] = i
		if err := checkCheckRule(r, fs, features); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	return nil
}

func checkCheckRule(r CheckRule, fs []CompiledFoundation, features []Feature) error {
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
		if err := checkWhen(w, fs, features); err != nil {
			return fmt.Errorf("when[%d]: %w", i, err)
		}
	}
	return checkDoc(r.Doc)
}

func checkWhen(w RuleWhen, fs []CompiledFoundation, features []Feature) error {
	var values []string
	if id, ok := strings.CutPrefix(w.Field, FeatureFieldPrefix); ok {
		if !slices.ContainsFunc(features, func(f Feature) bool { return f.ID == id }) {
			return fmt.Errorf("%q names no feature of the catalogue", w.Field)
		}
		values = []string{FeatureOn, FeatureOff}
	} else {
		var err error
		if _, values, err = addressable(w.Field, fs); err != nil {
			return err
		}
	}
	switch {
	case w.Unset && w.Equals != "":
		return fmt.Errorf("%s has both equals and unset", w.Field)
	case w.Unset && !strings.HasPrefix(w.Field, AnswerFieldPrefix):
		return fmt.Errorf("unset applies to %s<foundation id> only, not %s", AnswerFieldPrefix, w.Field)
	case w.Unset:
		return nil
	case w.Equals == "":
		return fmt.Errorf("%s has neither equals nor unset", w.Field)
	case values != nil && !slices.Contains(values, w.Equals):
		return fmt.Errorf("%q is not a value of %s (%s)", w.Equals, w.Field, strings.Join(values, ", "))
	}
	return nil
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
