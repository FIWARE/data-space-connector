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
	// Name is a template over answers; the requirement applies only while every one of them resolves.
	Name string `yaml:"name"`
	// Key is the data key the object must hold.
	Key     string `yaml:"key"`
	Purpose string `yaml:"purpose"`
}

// CompiledRequirement is a validated Requirement.
type CompiledRequirement struct {
	Kind, Key, Purpose string
	Name               *template.Template
	// Refs are the answer ids Name reads, sorted.
	Refs []string
}

// Requirements returns the bundle's requirements table in bundle order.
func (b Bundle) Requirements() []CompiledRequirement {
	return b.requirements
}

// Resolve renders the name over the resolved answers, keyed by foundation id; false while a ref is unanswered.
func (r CompiledRequirement) Resolve(answers map[string]any) (string, bool, error) {
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
	switch {
	case r.Kind != RequirementSecret:
		return CompiledRequirement{}, fmt.Errorf("kind %q is unknown; only %s is", r.Kind, RequirementSecret)
	case r.Key == "" || strings.Contains(r.Key, templateOpen):
		return CompiledRequirement{}, fmt.Errorf("key must be a non-empty literal")
	case strings.TrimSpace(r.Purpose) == "":
		return CompiledRequirement{}, fmt.Errorf("purpose is missing")
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
	return CompiledRequirement{Kind: r.Kind, Key: r.Key, Purpose: strings.TrimSpace(r.Purpose), Name: t, Refs: refs}, nil
}
