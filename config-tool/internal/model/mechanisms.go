package model

import (
	"errors"
	"fmt"
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// Mechanism declares when the render withholds a mechanism for something the chart templates itself, which no
// list's unique check sees (ADR 0025).
type Mechanism struct {
	ID string `yaml:"id"`
	// Conflicts each name what the chart templates and the mechanism would duplicate; any one of them withholds it.
	Conflicts []Conflict `yaml:"conflicts"`
}

// Conflict holds while a loaded file sets every WhenSet value path as a Helm `if` reads it: not absent, null,
// false, zero or empty.
type Conflict struct {
	WhenSet []string `yaml:"whenSet"`
}

// CompiledMechanism is a validated Mechanism.
type CompiledMechanism struct {
	ID        string
	Conflicts [][]roundtrip.Path
}

// Mechanisms returns the validated mechanisms table in bundle order.
func (b Bundle) Mechanisms() []CompiledMechanism {
	return b.mechanisms
}

// Conflict returns the paths of the first conflict that doc sets under root; false when none does.
func (m CompiledMechanism) Conflict(doc *roundtrip.Document, root roundtrip.Path) ([]string, bool) {
	for _, c := range m.Conflicts {
		if paths, ok := setAll(doc, root, c); ok {
			return paths, true
		}
	}
	return nil, false
}

// setAll returns ps as strings when doc sets every one of them under root.
func setAll(doc *roundtrip.Document, root roundtrip.Path, ps []roundtrip.Path) ([]string, bool) {
	var paths []string
	for _, p := range ps {
		n, err := doc.Lookup(slices.Concat(root, p))
		if err != nil {
			return nil, false
		}
		var v any
		if n.Decode(&v) != nil || !helmTruthy(v) {
			return nil, false
		}
		paths = append(paths, p.String())
	}
	return paths, true
}

// helmTruthy mirrors Go template truth, which a chart's `if` uses.
func helmTruthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case int:
		return v != 0
	case float64:
		return v != 0
	case map[string]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	}
	return true
}

// compileMechanisms validates the mechanisms table: distinct ids that element targets use, and non-empty conflicts
// whose paths sit under a top-level key and neither contain nor lie below a target, since only a loaded file sets them.
func compileMechanisms(ms []Mechanism, fs []CompiledFoundation, topLevelKeys []string) ([]CompiledMechanism, error) {
	var compiled []CompiledMechanism
	for i, m := range ms {
		c, err := compileMechanism(m, fs, topLevelKeys)
		if err == nil && slices.ContainsFunc(compiled, func(o CompiledMechanism) bool { return o.ID == m.ID }) {
			err = errors.New("id is already declared")
		}
		if err != nil {
			return nil, fmt.Errorf("mechanisms[%d] (%s): %w", i, m.ID, err)
		}
		compiled = append(compiled, c)
	}
	return compiled, nil
}

func compileMechanism(m Mechanism, fs []CompiledFoundation, topLevelKeys []string) (CompiledMechanism, error) {
	var targets []CompiledTarget
	for _, f := range fs {
		for _, t := range f.Targets {
			if t.Mechanism == m.ID {
				targets = append(targets, t)
			}
		}
	}
	switch {
	case m.ID == "":
		return CompiledMechanism{}, errors.New("id is missing")
	case len(targets) == 0:
		return CompiledMechanism{}, errors.New("no target uses the mechanism")
	case len(m.Conflicts) == 0:
		return CompiledMechanism{}, errors.New("conflicts is empty")
	}
	c := CompiledMechanism{ID: m.ID}
	for i, conflict := range m.Conflicts {
		if len(conflict.WhenSet) == 0 {
			return CompiledMechanism{}, fmt.Errorf("conflicts[%d]: whenSet is empty", i)
		}
		var compiled []roundtrip.Path
		for j, s := range conflict.WhenSet {
			p, err := compileConflictPath(s, fs, topLevelKeys)
			if err != nil {
				return CompiledMechanism{}, fmt.Errorf("conflicts[%d].whenSet[%d]: %w", i, j, err)
			}
			compiled = append(compiled, p)
		}
		c.Conflicts = append(c.Conflicts, compiled)
	}
	return c, nil
}

func compileConflictPath(s string, fs []CompiledFoundation, topLevelKeys []string) (roundtrip.Path, error) {
	p, err := roundtrip.ParsePath(s)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return nil, errors.New("path is missing")
	}
	if k, ok := p[0].Key(); !ok || !slices.Contains(topLevelKeys, k) {
		return nil, errors.New("root is not in topLevelKeys")
	}
	for _, f := range fs {
		for _, t := range f.Targets {
			if isPrefix(t.Path, p) || isPrefix(p, t.Path) {
				return nil, fmt.Errorf("%s writes %s", f.ID, t.Path)
			}
		}
	}
	return p, nil
}

// isPrefix reports whether p is a prefix of q.
func isPrefix(p, q roundtrip.Path) bool {
	return len(p) <= len(q) && slices.Equal(p, q[:len(p)])
}
