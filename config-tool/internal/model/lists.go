package model

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	// fullnameFunc is the one template function: Helm's standard fullname of a release's chart.
	fullnameFunc = "fullname"
	// maxResourceName is the Kubernetes limit Helm's fullname truncates to.
	maxResourceName = 63
	// templateOpen starts a template action; a string scalar without it is literal text.
	templateOpen = "{{"
)

// List declares a list that keyed element targets write into (ADR 0025).
type List struct {
	Path string `yaml:"path"`
	// Key is the field that identifies an element, such as name.
	Key string `yaml:"key"`
	// Unique are fields besides Key that Kubernetes requires to be unique in the list, such as mountPath.
	Unique []string `yaml:"unique"`
	// Base are the default elements of the layer that supplies the list; the render keeps them when no loaded file
	// holds it.
	Base []map[string]any `yaml:"base"`
}

// CompiledList is a validated List.
type CompiledList struct {
	Path   roundtrip.Path
	Key    string
	Unique []string
	Base   []map[string]any
}

// CompiledElement is a validated Element. Its value holds literal scalars, empty collections and parsed templates.
type CompiledElement struct {
	// KeyField is the list's key field and Key its value in this element.
	KeyField, Key string
	value         map[string]any
}

// Expand returns the element with every template executed by exec and the key field set; a fresh value per call.
func (e CompiledElement) Expand(exec func(*template.Template) (string, error)) (map[string]any, error) {
	v, err := expand(e.value, exec)
	if err != nil {
		return nil, err
	}
	m := v.(map[string]any)
	m[e.KeyField] = e.Key
	return m, nil
}

func expand(v any, exec func(*template.Template) (string, error)) (any, error) {
	switch v := v.(type) {
	case *template.Template:
		return exec(v)
	case map[string]any:
		m := make(map[string]any, len(v))
		for k, c := range v {
			x, err := expand(c, exec)
			if err != nil {
				return nil, err
			}
			m[k] = x
		}
		return m, nil
	case []any:
		s := make([]any, len(v))
		for i, c := range v {
			x, err := expand(c, exec)
			if err != nil {
				return nil, err
			}
			s[i] = x
		}
		return s, nil
	}
	return v, nil
}

// ListTable returns the validated lists table in bundle order.
func (b Bundle) ListTable() []CompiledList {
	return b.lists
}

// List returns the lists row declared for path; false when there is none.
func (b Bundle) List(path roundtrip.Path) (CompiledList, bool) {
	i := slices.IndexFunc(b.lists, func(l CompiledList) bool { return slices.Equal(l.Path, path) })
	if i < 0 {
		return CompiledList{}, false
	}
	return b.lists[i], true
}

// Fullname is Helm's standard fullname rule: release alone when it contains chart, else release-chart,
// truncated to 63 bytes without a trailing '-'.
func Fullname(release, chart string) string {
	name := release + "-" + chart
	if strings.Contains(release, chart) {
		name = release
	}
	if len(name) > maxResourceName {
		name = name[:maxResourceName]
	}
	return strings.TrimSuffix(name, "-")
}

// compileLists validates the lists table: paths of keys only under a top-level key, distinct and not nested, and
// base elements with distinct string keys.
func compileLists(ls []List, topLevelKeys []string) ([]CompiledList, error) {
	var compiled []CompiledList
	for i, l := range ls {
		c, err := compileList(l, topLevelKeys)
		if err != nil {
			return nil, fmt.Errorf("lists[%d] (%s): %w", i, l.Path, err)
		}
		for j, o := range compiled {
			if hasPathPrefix(c.Path, o.Path) || hasPathPrefix(o.Path, c.Path) {
				return nil, fmt.Errorf("lists[%d] (%s): overlaps lists[%d] (%s)", i, l.Path, j, o.Path)
			}
		}
		compiled = append(compiled, c)
	}
	return compiled, nil
}

func compileList(l List, topLevelKeys []string) (CompiledList, error) {
	p, err := roundtrip.ParsePath(l.Path)
	if err != nil {
		return CompiledList{}, err
	}
	if len(p) == 0 {
		return CompiledList{}, fmt.Errorf("path is missing")
	}
	if k, ok := p[0].Key(); !ok || !slices.Contains(topLevelKeys, k) {
		return CompiledList{}, fmt.Errorf("root is not in topLevelKeys")
	}
	if slices.ContainsFunc(p, func(s roundtrip.Segment) bool { _, ok := s.Index(); return ok }) {
		return CompiledList{}, fmt.Errorf("an index is not allowed in a list path")
	}
	if l.Key == "" {
		return CompiledList{}, fmt.Errorf("key is missing")
	}
	for i, u := range l.Unique {
		switch {
		case u == "":
			return CompiledList{}, fmt.Errorf("unique[%d] is empty", i)
		case u == l.Key:
			return CompiledList{}, fmt.Errorf("unique[%d]: %s is the key, which is unique anyway", i, u)
		case slices.Index(l.Unique, u) < i:
			return CompiledList{}, fmt.Errorf("unique[%d]: %s is already listed", i, u)
		}
	}
	seen := map[string]bool{}
	for i, e := range l.Base {
		k, ok := e[l.Key].(string)
		if !ok || k == "" {
			return CompiledList{}, fmt.Errorf("base[%d]: %s is not a non-empty string", i, l.Key)
		}
		if seen[k] {
			return CompiledList{}, fmt.Errorf("base[%d]: %s %q is already declared", i, l.Key, k)
		}
		seen[k] = true
	}
	return CompiledList{Path: p, Key: l.Key, Unique: l.Unique, Base: l.Base}, nil
}

// compileElementTarget compiles a target that writes one keyed element; the list's key field is filled in by
// checkElements, once the lists table is known.
func compileElementTarget(id string, t Target, c CompiledTarget) (CompiledTarget, error) {
	if t.Fn != "" || len(t.Args) > 0 || t.Sep != "" || t.Each != "" {
		return CompiledTarget{}, fmt.Errorf("an element target takes no fn, args, sep or each")
	}
	if t.Element.Key == "" || strings.Contains(t.Element.Key, templateOpen) {
		return CompiledTarget{}, fmt.Errorf("element: key must be a non-empty literal")
	}
	if len(t.Element.Value) == 0 {
		return CompiledTarget{}, fmt.Errorf("element: value is missing")
	}
	refs := map[string]bool{}
	v, err := compileValue(fmt.Sprintf("%s:%s[%s]", id, t.Path, t.Element.Key), t.Element.Value, refs)
	if err != nil {
		return CompiledTarget{}, fmt.Errorf("element: %w", err)
	}
	if len(refs) == 0 && len(c.Requires) == 0 {
		return CompiledTarget{}, fmt.Errorf("element references no answer, so it needs requires")
	}
	c.Element = &CompiledElement{Key: t.Element.Key, value: v.(map[string]any)}
	c.Refs = sortedSet(refs)
	return c, nil
}

// compileValue parses every string scalar of v that holds an action as a template and adds its refs to refs.
func compileValue(name string, v any, refs map[string]bool) (any, error) {
	switch v := v.(type) {
	case string:
		if !strings.Contains(v, templateOpen) {
			return v, nil
		}
		tpl, r, err := parseTemplate(name, v)
		if err != nil {
			return nil, err
		}
		if _, reserved := splitReserved(r); len(reserved) > 0 {
			return nil, fmt.Errorf("%s references the reserved %s, which only a default may read", name, reserved[0])
		}
		for _, id := range r {
			refs[id] = true
		}
		return tpl, nil
	case map[string]any:
		m := make(map[string]any, len(v))
		for _, k := range slices.Sorted(maps.Keys(v)) {
			c, err := compileValue(name+"."+k, v[k], refs)
			if err != nil {
				return nil, err
			}
			m[k] = c
		}
		return m, nil
	case []any:
		s := make([]any, len(v))
		for i, c := range v {
			x, err := compileValue(fmt.Sprintf("%s[%d]", name, i), c, refs)
			if err != nil {
				return nil, err
			}
			s[i] = x
		}
		return s, nil
	case nil:
		return nil, fmt.Errorf("%s is null, which the chart would read as unset", name)
	}
	if !isScalar(v) {
		return nil, fmt.Errorf("%s holds a %T", name, v)
	}
	return v, nil
}

// checkElements ties element and each targets to the lists table: an element writes into a declared list, under
// a key no other element or base element has, without its key field in the value and without sharing a unique
// value with another owned or a base element; each is refused on a declared list; every list has an element; a
// mechanism's targets all write elements.
func checkElements(fs []CompiledFoundation, lists []CompiledList) error {
	type owned struct {
		key, where string
		value      map[string]any
	}
	byList := make([][]owned, len(lists))
	for i := range fs {
		for j := range fs[i].Targets {
			t := &fs[i].Targets[j]
			where := fmt.Sprintf("foundations[%d] (%s): targets[%d]", i, fs[i].ID, j)
			k := slices.IndexFunc(lists, func(l CompiledList) bool { return slices.Equal(l.Path, t.Path) })
			if t.Each != nil && k >= 0 {
				return fmt.Errorf("%s: each is not allowed on %q, which the lists table declares for element targets", where, t.Path)
			}
			if t.Element == nil {
				continue
			}
			if k < 0 {
				return fmt.Errorf("%s: element list %q is not declared in the lists table", where, t.Path)
			}
			l := lists[k]
			t.Element.KeyField = l.Key
			if _, ok := t.Element.value[l.Key]; ok {
				return fmt.Errorf("%s: element value holds the key field %s; element.key sets it", where, l.Key)
			}
			if b := slices.IndexFunc(l.Base, func(e map[string]any) bool { return e[l.Key] == any(t.Element.Key) }); b >= 0 {
				return fmt.Errorf("%s: element %s is base[%d] of list %q", where, t.Element.Key, b, t.Path)
			}
			for _, o := range byList[k] {
				if o.key == t.Element.Key {
					return fmt.Errorf("%s: element %s of list %q is already written by %s", where, t.Element.Key, t.Path, o.where)
				}
			}
			for _, u := range l.Unique {
				v, ok := t.Element.value[u]
				if !ok {
					continue
				}
				if _, isTpl := v.(*template.Template); isTpl || !isScalar(v) {
					return fmt.Errorf("%s: element %s: unique field %s must be a literal scalar", where, t.Element.Key, u)
				}
				for _, o := range byList[k] {
					if o.value[u] == v {
						return fmt.Errorf("%s: element %s shares %s %v with %s", where, t.Element.Key, u, v, o.where)
					}
				}
				if b := slices.IndexFunc(l.Base, func(e map[string]any) bool { return e[u] == v }); b >= 0 {
					return fmt.Errorf("%s: element %s shares %s %v with base[%d] of list %q", where, t.Element.Key, u, v, b, t.Path)
				}
			}
			byList[k] = append(byList[k], owned{key: t.Element.Key, where: where, value: t.Element.value})
		}
	}
	for k, l := range lists {
		if len(byList[k]) == 0 {
			return fmt.Errorf("lists[%d] (%s): no element target writes it", k, l.Path)
		}
	}
	return nil
}
