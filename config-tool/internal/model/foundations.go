package model

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// FoundationType is the value type of a foundation answer; each has one pattern.
type FoundationType string

// Foundation types.
const (
	// TypeString is a non-empty single line without leading or trailing whitespace, control characters or
	// any of " \ < >, because chart templates interpolate it unescaped into JSON and HTML (_realm.tpl).
	TypeString FoundationType = "string"
	// TypeHostname is a DNS-1123 subdomain, as an ingress host must be.
	TypeHostname FoundationType = "hostname"
	// TypeDomain is a hostname of at least two labels, the base other hosts are derived from.
	TypeDomain FoundationType = "domain"
)

// Fn is a target function: how a target's value is made from answers (ADR 0020).
type Fn string

// Target functions; the bundle header documents each.
const (
	// FnTpl renders its one template arg.
	FnTpl Fn = "tpl"
	// FnJoin renders every template arg and joins the results with Sep.
	FnJoin Fn = "join"
	// FnCopy takes the one answer its template arg names, keeping the answer's type.
	FnCopy Fn = "copy"
	// FnConst writes its one scalar arg verbatim; only inside an owned list.
	FnConst Fn = "const"
)

// maxHostnameLength is the DNS limit on a full host name.
const maxHostnameLength = 253

var (
	knownTypes = []FoundationType{TypeString, TypeHostname, TypeDomain}
	knownFns   = []Fn{FnTpl, FnJoin, FnCopy, FnConst}
	// idSegment keeps every segment of an answer id usable as a template field name.
	idSegment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	plainLine = regexp.MustCompile(`^[^\s\x00-\x1f\x7f"\\<>]([^\x00-\x1f\x7f"\\<>]*[^\s\x00-\x1f\x7f"\\<>])?$`)
	hostname  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

// Foundation is a cross-cutting answer as declared in a bundle's foundations list.
type Foundation struct {
	// ID is dotted (domain.base); it keys profile.Answers.
	ID    string         `yaml:"id"`
	Stage Stage          `yaml:"stage"`
	Type  FoundationType `yaml:"type"`
	// Default is a template over other answers; a defaulted answer exists only when all of them do.
	Default string `yaml:"default"`
	Label   string `yaml:"label"`
	Help    string `yaml:"help"`
	// Doc is a repo-relative documentation path, optionally with an anchor.
	Doc     string   `yaml:"doc"`
	Targets []Target `yaml:"targets"`
}

// Target is one chart key a foundation writes.
type Target struct {
	// Path is in roundtrip.ParsePath syntax; an index is allowed only into a list the bundle owns whole.
	Path string `yaml:"path"`
	Fn   Fn     `yaml:"fn"`
	// Args are templates for tpl, join and copy, and the one scalar of const.
	Args []any  `yaml:"args"`
	Sep  string `yaml:"sep"`
}

// CompiledFoundation is a validated Foundation with its templates parsed.
type CompiledFoundation struct {
	ID    string
	Stage Stage
	Type  FoundationType
	// Refs are the answer ids Default references, sorted; nil without a default.
	Refs    []string
	Targets []CompiledTarget
	deflt   *template.Template
}

// DefaultTemplate is the parsed Default; nil when the foundation has none.
func (f CompiledFoundation) DefaultTemplate() *template.Template {
	return f.deflt
}

// CompiledTarget is a validated Target.
type CompiledTarget struct {
	Path roundtrip.Path
	Fn   Fn
	// Value is the scalar of a const target; nil otherwise.
	Value any
	Sep   string
	// Refs are the answer ids the target references, sorted; empty for const.
	Refs []string
	tpls []*template.Template
}

// Templates are the parsed args of a tpl, join or copy target, in order; nil for const.
func (t CompiledTarget) Templates() []*template.Template {
	return t.tpls
}

// Foundation returns the compiled foundation id; false when the bundle does not declare it.
func (b Bundle) Foundation(id string) (CompiledFoundation, bool) {
	i := slices.IndexFunc(b.foundations, func(f CompiledFoundation) bool { return f.ID == id })
	if i < 0 {
		return CompiledFoundation{}, false
	}
	return b.foundations[i], true
}

// FoundationOrder lists the foundation ids so that each comes after the answers its default
// references; ties keep declaration order.
func (b Bundle) FoundationOrder() []string {
	return slices.Clone(b.order)
}

// Check returns why v is not a valid answer of type t, without echoing v; nil when it is valid.
func (t FoundationType) Check(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	switch t {
	case TypeString:
		if !plainLine.MatchString(s) {
			return fmt.Errorf(`must be a single non-empty line without leading or trailing spaces, control characters, '"', '\', '<' or '>'`)
		}
	case TypeHostname, TypeDomain:
		if len(s) > maxHostnameLength || !hostname.MatchString(s) {
			return fmt.Errorf("must be a host name of lowercase letters, digits, '-' and '.', at most %d characters", maxHostnameLength)
		}
		if t == TypeDomain && !strings.Contains(s, ".") {
			return fmt.Errorf("must be a domain of at least two labels")
		}
	default:
		return fmt.Errorf("type %q is unknown", t)
	}
	return nil
}

// compileFoundations validates the foundations list and returns it compiled with its topological order.
func compileFoundations(fs []Foundation, topLevelKeys []string) ([]CompiledFoundation, []string, error) {
	if len(fs) == 0 {
		return nil, nil, nil
	}
	compiled := make([]CompiledFoundation, len(fs))
	for i, f := range fs {
		c, err := compileFoundation(f, topLevelKeys)
		if err != nil {
			return nil, nil, fmt.Errorf("foundations[%d] (%s): %w", i, f.ID, err)
		}
		if j := slices.IndexFunc(compiled[:i], func(o CompiledFoundation) bool { return o.ID == f.ID }); j >= 0 {
			return nil, nil, fmt.Errorf("foundations[%d]: id %q is already declared by foundations[%d]", i, f.ID, j)
		}
		compiled[i] = c
	}
	if err := checkIDNesting(compiled); err != nil {
		return nil, nil, err
	}
	if err := checkRefs(compiled); err != nil {
		return nil, nil, err
	}
	if err := checkTargetPaths(compiled); err != nil {
		return nil, nil, err
	}
	order, err := topoOrder(compiled)
	if err != nil {
		return nil, nil, err
	}
	return compiled, order, nil
}

func compileFoundation(f Foundation, topLevelKeys []string) (CompiledFoundation, error) {
	if err := checkID(f.ID); err != nil {
		return CompiledFoundation{}, err
	}
	if !slices.Contains(knownStages, f.Stage) {
		return CompiledFoundation{}, fmt.Errorf("stage %q is unknown", f.Stage)
	}
	if !slices.Contains(knownTypes, f.Type) {
		return CompiledFoundation{}, fmt.Errorf("type %q is unknown", f.Type)
	}
	if f.Label == "" {
		return CompiledFoundation{}, fmt.Errorf("label is missing")
	}
	c := CompiledFoundation{ID: f.ID, Stage: f.Stage, Type: f.Type}
	if f.Default != "" {
		t, refs, err := parseTemplate(f.ID, f.Default)
		if err != nil {
			return CompiledFoundation{}, fmt.Errorf("default: %w", err)
		}
		if len(refs) == 0 {
			return CompiledFoundation{}, fmt.Errorf("default references no answer")
		}
		c.deflt, c.Refs = t, refs
	}
	for i, t := range f.Targets {
		ct, err := compileTarget(f.ID, t, topLevelKeys)
		if err != nil {
			return CompiledFoundation{}, fmt.Errorf("targets[%d]: %w", i, err)
		}
		c.Targets = append(c.Targets, ct)
	}
	return c, nil
}

func checkID(id string) error {
	if id == "" {
		return fmt.Errorf("id is missing")
	}
	for _, s := range strings.Split(id, ".") {
		if !idSegment.MatchString(s) {
			return fmt.Errorf("id %q: segment %q is not a letter or '_' followed by letters, digits and '_'", id, s)
		}
	}
	return nil
}

func compileTarget(id string, t Target, topLevelKeys []string) (CompiledTarget, error) {
	p, err := roundtrip.ParsePath(t.Path)
	if err != nil {
		return CompiledTarget{}, err
	}
	if len(p) == 0 {
		return CompiledTarget{}, fmt.Errorf("path is missing")
	}
	if k, ok := p[0].Key(); !ok || !slices.Contains(topLevelKeys, k) {
		return CompiledTarget{}, fmt.Errorf("path %q: root is not in topLevelKeys", t.Path)
	}
	c := CompiledTarget{Path: p, Fn: t.Fn, Sep: t.Sep}
	if t.Sep != "" && t.Fn != FnJoin {
		return CompiledTarget{}, fmt.Errorf("sep is only allowed on %s targets", FnJoin)
	}
	switch t.Fn {
	case FnConst:
		if !slices.ContainsFunc(p, func(s roundtrip.Segment) bool { _, ok := s.Index(); return ok }) {
			return CompiledTarget{}, fmt.Errorf("%s is only allowed under a list index, which completes an owned list", FnConst)
		}
		if len(t.Args) != 1 || !isScalar(t.Args[0]) {
			return CompiledTarget{}, fmt.Errorf("%s takes exactly one scalar arg", FnConst)
		}
		if s, ok := t.Args[0].(string); ok && strings.Contains(s, "{{") {
			return CompiledTarget{}, fmt.Errorf("%s is written verbatim and cannot reference an answer", FnConst)
		}
		c.Value = t.Args[0]
		return c, nil
	case FnTpl, FnCopy:
		if len(t.Args) != 1 {
			return CompiledTarget{}, fmt.Errorf("%s takes exactly one template arg", t.Fn)
		}
	case FnJoin:
		if len(t.Args) == 0 {
			return CompiledTarget{}, fmt.Errorf("%s takes at least one template arg", t.Fn)
		}
	default:
		return CompiledTarget{}, fmt.Errorf("fn %q is unknown", t.Fn)
	}
	refs := map[string]bool{}
	for i, a := range t.Args {
		s, ok := a.(string)
		if !ok {
			return CompiledTarget{}, fmt.Errorf("args[%d] is not a template string", i)
		}
		tpl, r, err := parseTemplate(fmt.Sprintf("%s:%s", id, t.Path), s)
		if err != nil {
			return CompiledTarget{}, fmt.Errorf("args[%d]: %w", i, err)
		}
		if len(r) == 0 {
			return CompiledTarget{}, fmt.Errorf("args[%d] references no answer", i)
		}
		if t.Fn == FnCopy && !isSingleField(tpl) {
			return CompiledTarget{}, fmt.Errorf("%s takes one {{ .id }} action and no text", FnCopy)
		}
		c.tpls = append(c.tpls, tpl)
		for _, id := range r {
			refs[id] = true
		}
	}
	c.Refs = sortedSet(refs)
	return c, nil
}

func isScalar(v any) bool {
	switch v.(type) {
	case string, bool, int, float64:
		return true
	}
	return false
}

// parseTemplate parses s and returns the answer ids it references. Only text and {{ .id }} actions
// are allowed, so the references read from the parse tree are complete.
func parseTemplate(name, s string) (*template.Template, []string, error) {
	t, err := template.New(name).Option("missingkey=error").Funcs(template.FuncMap{}).Parse(s)
	if err != nil {
		return nil, nil, err
	}
	refs := map[string]bool{}
	for _, n := range t.Root.Nodes {
		switch n := n.(type) {
		case *parse.TextNode:
		case *parse.ActionNode:
			f := fieldOf(n)
			if f == nil {
				return nil, nil, fmt.Errorf("action %s: only {{ .id }} is allowed", n)
			}
			refs[strings.Join(f.Ident, ".")] = true
		default:
			return nil, nil, fmt.Errorf("%s: only text and {{ .id }} actions are allowed", n)
		}
	}
	return t, sortedSet(refs), nil
}

// fieldOf returns the field of an action that is exactly {{ .a.b }}; nil otherwise.
func fieldOf(n *parse.ActionNode) *parse.FieldNode {
	if len(n.Pipe.Decl) > 0 || len(n.Pipe.Cmds) != 1 || len(n.Pipe.Cmds[0].Args) != 1 {
		return nil
	}
	f, _ := n.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
	return f
}

func isSingleField(t *template.Template) bool {
	nodes := t.Root.Nodes
	if len(nodes) != 1 {
		return false
	}
	a, ok := nodes[0].(*parse.ActionNode)
	return ok && fieldOf(a) != nil
}

// checkIDNesting refuses an id that is a segment prefix of another: the template namespace nests ids,
// so domain could not be both a value and the parent of domain.base.
func checkIDNesting(fs []CompiledFoundation) error {
	for i, f := range fs {
		for j, o := range fs {
			if strings.HasPrefix(o.ID, f.ID+".") {
				return fmt.Errorf("foundations[%d]: id %q is a prefix of foundations[%d] %q", i, f.ID, j, o.ID)
			}
		}
	}
	return nil
}

// checkRefs refuses a reference to an undeclared id; topoOrder refuses a default that references its own.
func checkRefs(fs []CompiledFoundation) error {
	declared := func(id string) bool {
		return slices.ContainsFunc(fs, func(f CompiledFoundation) bool { return f.ID == id })
	}
	for i, f := range fs {
		for _, r := range f.Refs {
			if !declared(r) {
				return fmt.Errorf("foundations[%d] (%s): default references unknown answer %q", i, f.ID, r)
			}
		}
		for j, t := range f.Targets {
			for _, r := range t.Refs {
				if !declared(r) {
					return fmt.Errorf("foundations[%d] (%s): targets[%d] references unknown answer %q", i, f.ID, j, r)
				}
			}
		}
	}
	return nil
}

// checkTargetPaths refuses a path written twice, a path that is a prefix of another (every leaf is a
// scalar), a prefix used both as a list and as a mapping, and a list whose indices are not 0..n-1.
func checkTargetPaths(fs []CompiledFoundation) error {
	type where struct{ f, t int }
	leaves := map[string]where{}
	// interior maps every proper prefix to whether its child segment is an index.
	interior := map[string]bool{}
	indices := map[string]map[int]bool{}
	var lists []string
	for i, f := range fs {
		for j, t := range f.Targets {
			key := t.Path.String()
			if w, dup := leaves[key]; dup {
				return fmt.Errorf("foundations[%d] (%s): targets[%d]: path %q is already written by foundations[%d] targets[%d]", i, f.ID, j, key, w.f, w.t)
			}
			leaves[key] = where{i, j}
			for k := 1; k < len(t.Path); k++ {
				prefix := t.Path[:k].String()
				idx, isIndex := t.Path[k].Index()
				if was, seen := interior[prefix]; seen && was != isIndex {
					return fmt.Errorf("foundations[%d] (%s): targets[%d]: %q is used both as a list and as a mapping", i, f.ID, j, prefix)
				}
				interior[prefix] = isIndex
				if isIndex {
					if indices[prefix] == nil {
						indices[prefix] = map[int]bool{}
						lists = append(lists, prefix)
					}
					indices[prefix][idx] = true
				}
			}
		}
	}
	for key, w := range leaves {
		if _, ok := interior[key]; ok {
			return fmt.Errorf("foundations[%d] (%s): targets[%d]: path %q is a prefix of another target's path", w.f, fs[w.f].ID, w.t, key)
		}
	}
	for _, l := range lists {
		for i := range len(indices[l]) {
			if !indices[l][i] {
				return fmt.Errorf("list %q: index %d is missing; an owned list is written whole, from index 0 without gaps", l, i)
			}
		}
	}
	return nil
}

// topoOrder orders ids after the ids their default references, in declaration order otherwise.
func topoOrder(fs []CompiledFoundation) ([]string, error) {
	placed := map[string]bool{}
	order := make([]string, 0, len(fs))
	for len(order) < len(fs) {
		progress := false
		for _, f := range fs {
			ready := !slices.ContainsFunc(f.Refs, func(r string) bool { return !placed[r] })
			if placed[f.ID] || !ready {
				continue
			}
			placed[f.ID] = true
			order = append(order, f.ID)
			progress = true
			break
		}
		if !progress {
			var cycle []string
			for _, f := range fs {
				if !placed[f.ID] {
					cycle = append(cycle, f.ID)
				}
			}
			return nil, fmt.Errorf("foundations: the defaults of %s reference each other in a cycle", strings.Join(cycle, ", "))
		}
	}
	return order, nil
}

// sortedSet returns the keys of m sorted; nil when m is empty.
func sortedSet(m map[string]bool) []string {
	return slices.Sorted(maps.Keys(m))
}
