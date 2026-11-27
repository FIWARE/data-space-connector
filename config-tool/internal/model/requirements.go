package model

import (
	"fmt"
	"slices"
	"strings"
	"text/template"
)

// RequirementSecret is the only requirement kind: a Secret the user creates before installing.
const RequirementSecret = "secret"

// Requirement is a cluster object the rendered values reference but the tool never creates; NEXT_STEPS lists it.
type Requirement struct {
	Kind string `yaml:"kind"`
	// Name is a template over answers; the requirement applies only while every one of them resolves. A per-ingress
	// question's requirement has none: its name is the question's answer.
	Name string `yaml:"name"`
	// Key is the data key the object must hold; Keys lists several. Exactly one of them is set.
	Key     string   `yaml:"key"`
	Keys    []string `yaml:"keys"`
	Purpose string   `yaml:"purpose"`
	// When must all hold, over compiled answer ids, for the requirement to apply; a mode's rows get its mode too.
	When []TargetCondition `yaml:"when"`
}

// CompiledRequirement is a validated Requirement.
type CompiledRequirement struct {
	Kind, Purpose string
	// Keys are the data keys the object must hold, in declaration order.
	Keys []string
	Name *template.Template
	// Refs are the answer ids Name reads, sorted.
	Refs []string
	// When must all hold and Requires resolve for the requirement to apply; both nil for an ungated row.
	When     []TargetCondition
	Requires []string
}

// Requirements returns the bundle's requirements table in bundle order.
func (b Bundle) Requirements() []CompiledRequirement {
	return b.requirements
}

// Resolve renders the name over the resolved answers, keyed by foundation id; false while a ref or a requires is
// unanswered or a When condition does not hold.
func (r CompiledRequirement) Resolve(answers map[string]any) (string, bool, error) {
	if slices.ContainsFunc(r.When, func(c TargetCondition) bool { return !c.Holds(answers) }) {
		return "", false, nil
	}
	if slices.ContainsFunc(r.Requires, func(id string) bool { _, ok := answers[id]; return !ok }) {
		return "", false, nil
	}
	ns := map[string]any{}
	for _, id := range r.Refs {
		v, ok := answers[id]
		if !ok {
			return "", false, nil
		}
		SetNested(ns, id, v)
	}
	s, err := Execute(r.Name, ns)
	return s, err == nil, err
}

// compileRequirements validates the requirements table against the compiled foundations.
func compileRequirements(rs []Requirement, fs []CompiledFoundation) ([]CompiledRequirement, error) {
	var compiled []CompiledRequirement
	for i, r := range rs {
		c, err := compileRequirement(i, r, fs)
		if err != nil {
			return nil, fmt.Errorf("requirements[%d] (%s): %w", i, r.Name, err)
		}
		compiled = append(compiled, c)
	}
	return compiled, nil
}

func compileRequirement(i int, r Requirement, fs []CompiledFoundation) (CompiledRequirement, error) {
	keys := r.Keys
	if r.Key != "" {
		keys = []string{r.Key}
	}
	switch {
	case r.Kind != RequirementSecret:
		return CompiledRequirement{}, fmt.Errorf("kind %q is unknown; only %s is", r.Kind, RequirementSecret)
	case r.Key != "" && len(r.Keys) > 0:
		return CompiledRequirement{}, fmt.Errorf("key and keys are exclusive")
	case len(keys) == 0 || slices.ContainsFunc(keys, func(k string) bool { return k == "" || strings.Contains(k, templateOpen) }):
		return CompiledRequirement{}, fmt.Errorf("key must be a non-empty literal")
	case len(slices.Compact(slices.Sorted(slices.Values(keys)))) != len(keys):
		return CompiledRequirement{}, fmt.Errorf("keys lists a key twice")
	case strings.TrimSpace(r.Purpose) == "":
		return CompiledRequirement{}, fmt.Errorf("purpose is missing")
	}
	for _, c := range r.When {
		j := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == c.ID })
		switch {
		case c.Equals == "":
			return CompiledRequirement{}, fmt.Errorf("when: %s: equals is missing", c.ID)
		case j < 0:
			return CompiledRequirement{}, fmt.Errorf("when: %s is no foundation", c.ID)
		case !fs[j].Type.IsString():
			// TargetCondition.Holds compares with a string, so a condition on a bool or a list never holds.
			return CompiledRequirement{}, fmt.Errorf("when: %s is a %s; a condition compares a string answer", c.ID, fs[j].Type)
		case fs[j].Type == TypeChoice && !slices.Contains(fs[j].optionValues(), c.Equals):
			return CompiledRequirement{}, fmt.Errorf("when: %q is not an option of %s", c.Equals, c.ID)
		}
	}
	t, refs, err := parseTemplate(fmt.Sprintf("requirements[%d]", i), r.Name)
	if err != nil {
		return CompiledRequirement{}, err
	}
	if len(refs) == 0 {
		return CompiledRequirement{}, fmt.Errorf("name references no answer")
	}
	if _, reserved := splitReserved(refs); len(reserved) > 0 {
		return CompiledRequirement{}, fmt.Errorf("name references the reserved %s, which only a default may read", reserved[0])
	}
	for _, id := range refs {
		if !slices.ContainsFunc(fs, func(f CompiledFoundation) bool { return f.ID == id }) {
			return CompiledRequirement{}, fmt.Errorf("name references %s, which is no foundation", id)
		}
	}
	return CompiledRequirement{Kind: r.Kind, Keys: slices.Clone(keys), Purpose: strings.TrimSpace(r.Purpose), Name: t, Refs: refs,
		When: slices.Clone(r.When)}, nil
}
