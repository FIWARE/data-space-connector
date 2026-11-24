// Package check evaluates a bundle's L2 rules over a profile (ADR 0021); only error findings block generate.
package check

import (
	"cmp"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// Finding is one rule that holds for a profile, addressed by the open-field id it points at.
type Finding struct {
	Rule     string         `json:"rule"`
	Severity model.Severity `json:"severity"`
	Stage    model.Stage    `json:"stage"`
	Field    string         `json:"field"`
	Message  string         `json:"message"`
	Doc      string         `json:"doc"`
}

// Evaluate returns the findings of b's rules over p, sorted by severity, stage and rule id. Conditions
// read resolved answers, so a derived default counts; it errors when the answers do not resolve.
func Evaluate(b model.Bundle, p profile.Profile) ([]Finding, error) {
	answers, err := render.Answers(b, p)
	if err != nil {
		return nil, err
	}
	findings := []Finding{}
	for _, r := range b.CheckRules() {
		if !slices.ContainsFunc(r.When, func(w model.RuleWhen) bool { return !holds(w, p, answers) }) {
			findings = append(findings, Finding{
				Rule: r.ID, Severity: r.Severity, Stage: r.Stage, Field: r.Field, Message: r.Message, Doc: r.Doc,
			})
		}
	}
	slices.SortFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.Severity.Rank(), b.Severity.Rank()),
			cmp.Compare(a.Stage.Rank(), b.Stage.Rank()),
			strings.Compare(a.Rule, b.Rule),
		)
	})
	return findings, nil
}

// Blocking reports whether any finding is an error, which stops generate.
func Blocking(findings []Finding) bool {
	return slices.ContainsFunc(findings, func(f Finding) bool { return f.Severity == model.SeverityError })
}

// holds reports whether w's field resolves to w.Equals; an unanswered or non-string field never does.
func holds(w model.RuleWhen, p profile.Profile, answers map[string]any) bool {
	v, ok := value(w.Field, p, answers)
	return ok && v == w.Equals
}

// value reads a field the bundle validated as addressable.
func value(field string, p profile.Profile, answers map[string]any) (string, bool) {
	if field == model.FieldTargetEnvironment {
		return string(p.Target.Environment), p.Target.Environment != ""
	}
	id, _ := strings.CutPrefix(field, model.AnswerFieldPrefix)
	s, ok := answers[id].(string)
	return s, ok
}
